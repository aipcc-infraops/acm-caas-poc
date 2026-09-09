package lifecycle

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/pablofelix/acm-caas-poc/internal/client"
)

type RecoveryResult struct {
	CSRsApproved int      `json:"csrsApproved"`
	CSRNames     []string `json:"csrNames,omitempty"`
	Message      string   `json:"message"`
}

// PostResumeRecovery connects to the spoke cluster and approves any expired
// kubelet certificates that accumulated while the cluster was hibernated.
// Kubelet client certs rotate every ~24h in OpenShift; if the cluster was
// powered off during rotation, the certs expire and must be approved manually.
func (m *Manager) PostResumeRecovery(ctx context.Context, namespace, name string) (*RecoveryResult, error) {
	spokeConfig, err := m.getSpokeRESTConfig(ctx, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting spoke kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(spokeConfig)
	if err != nil {
		return nil, fmt.Errorf("creating spoke clientset: %w", err)
	}

	total := &RecoveryResult{}

	// CSRs arrive in waves: client certs first, then serving certs after
	// the client certs are approved. Run up to 3 rounds.
	for round := 0; round < 3; round++ {
		if round > 0 {
			select {
			case <-ctx.Done():
				return total, ctx.Err()
			case <-time.After(15 * time.Second):
			}
		}

		result, err := approveExpiredCSRs(ctx, clientset)
		if err != nil {
			return total, fmt.Errorf("approving CSRs (round %d): %w", round+1, err)
		}

		total.CSRsApproved += result.CSRsApproved
		total.CSRNames = append(total.CSRNames, result.CSRNames...)

		if result.CSRsApproved == 0 {
			break
		}
	}

	if total.CSRsApproved > 0 {
		total.Message = fmt.Sprintf("Approved %d expired kubelet certificate(s)", total.CSRsApproved)
	} else {
		total.Message = "No expired certificates found"
	}

	return total, nil
}

func (m *Manager) getSpokeRESTConfig(ctx context.Context, namespace, name string) (*rest.Config, error) {
	cd, err := m.client.Get(ctx, client.GVRClusterDeployment, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("getting ClusterDeployment: %w", err)
	}

	secretName, found, _ := unstructured.NestedString(cd.Object, "status", "adminKubeconfigSecretRef", "name")
	if !found || secretName == "" {
		return nil, fmt.Errorf("ClusterDeployment %s/%s has no adminKubeconfigSecretRef", namespace, name)
	}

	secret, err := m.client.Get(ctx, client.GVRSecret, namespace, secretName)
	if err != nil {
		return nil, fmt.Errorf("getting admin kubeconfig secret %s: %w", secretName, err)
	}

	kubeconfigB64, found, _ := unstructured.NestedString(secret.Object, "data", "kubeconfig")
	if !found {
		return nil, fmt.Errorf("secret %s has no kubeconfig data", secretName)
	}

	kubeconfigBytes, err := base64.StdEncoding.DecodeString(kubeconfigB64)
	if err != nil {
		return nil, fmt.Errorf("decoding kubeconfig: %w", err)
	}

	return clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
}

func approveExpiredCSRs(ctx context.Context, clientset kubernetes.Interface) (*RecoveryResult, error) {
	csrClient := clientset.CertificatesV1().CertificateSigningRequests()

	csrList, err := csrClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing CSRs: %w", err)
	}

	result := &RecoveryResult{}

	for i := range csrList.Items {
		csr := &csrList.Items[i]
		if !isPendingKubeletCSR(*csr) {
			continue
		}

		csr.Status.Conditions = append(csr.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
			Type:               certificatesv1.CertificateApproved,
			Status:             corev1.ConditionTrue,
			Reason:             "AutoApprovedByAcmlab",
			Message:            "Approved by acmlab post-resume recovery",
			LastUpdateTime:     metav1.Now(),
		})

		_, err := csrClient.UpdateApproval(ctx, csr.Name, csr, metav1.UpdateOptions{})
		if err != nil {
			continue
		}

		result.CSRsApproved++
		result.CSRNames = append(result.CSRNames, csr.Name)
	}

	return result, nil
}

func isPendingKubeletCSR(csr certificatesv1.CertificateSigningRequest) bool {
	kubeletSigners := map[string]bool{
		"kubernetes.io/kube-apiserver-client-kubelet": true,
		"kubernetes.io/kubelet-serving":               true,
	}

	if !kubeletSigners[csr.Spec.SignerName] {
		return false
	}

	for _, condition := range csr.Status.Conditions {
		if condition.Type == certificatesv1.CertificateApproved || condition.Type == certificatesv1.CertificateDenied {
			return false
		}
	}

	return true
}
