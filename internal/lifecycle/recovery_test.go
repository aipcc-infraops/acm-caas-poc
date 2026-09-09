package lifecycle

import (
	"testing"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakekube "k8s.io/client-go/kubernetes/fake"
)

func TestIsPendingKubeletCSRReturnsTrueForClientKubelet(t *testing.T) {
	csr := certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{
			SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
		},
	}
	if !isPendingKubeletCSR(csr) {
		t.Error("expected true for pending kube-apiserver-client-kubelet CSR")
	}
}

func TestIsPendingKubeletCSRReturnsTrueForServing(t *testing.T) {
	csr := certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{
			SignerName: "kubernetes.io/kubelet-serving",
		},
	}
	if !isPendingKubeletCSR(csr) {
		t.Error("expected true for pending kubelet-serving CSR")
	}
}

func TestIsPendingKubeletCSRReturnsFalseForNonKubelet(t *testing.T) {
	csr := certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{
			SignerName: "kubernetes.io/kube-apiserver-client",
		},
	}
	if isPendingKubeletCSR(csr) {
		t.Error("expected false for non-kubelet CSR")
	}
}

func TestIsPendingKubeletCSRReturnsFalseWhenAlreadyApproved(t *testing.T) {
	csr := certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{
			SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1.CertificateSigningRequestCondition{
				{
					Type:   certificatesv1.CertificateApproved,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	if isPendingKubeletCSR(csr) {
		t.Error("expected false for already approved CSR")
	}
}

func TestIsPendingKubeletCSRReturnsFalseWhenDenied(t *testing.T) {
	csr := certificatesv1.CertificateSigningRequest{
		Spec: certificatesv1.CertificateSigningRequestSpec{
			SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1.CertificateSigningRequestCondition{
				{
					Type:   certificatesv1.CertificateDenied,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
	if isPendingKubeletCSR(csr) {
		t.Error("expected false for denied CSR")
	}
}

func TestApproveExpiredCSRsWithFakeClient(t *testing.T) {
	ctx := t.Context()

	pendingCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "csr-test1"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
			Request:    []byte("fake"),
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth},
		},
	}
	approvedCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "csr-test2"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			SignerName: "kubernetes.io/kube-apiserver-client-kubelet",
			Request:    []byte("fake"),
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth},
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1.CertificateSigningRequestCondition{
				{Type: certificatesv1.CertificateApproved, Status: corev1.ConditionTrue},
			},
		},
	}
	nonKubeletCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "csr-test3"},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			SignerName: "kubernetes.io/kube-apiserver-client",
			Request:    []byte("fake"),
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth},
		},
	}

	clientset := fakekube.NewSimpleClientset(pendingCSR, approvedCSR, nonKubeletCSR)

	result, err := approveExpiredCSRs(ctx, clientset)
	if err != nil {
		t.Fatalf("approveExpiredCSRs() error = %v", err)
	}

	if result.CSRsApproved != 1 {
		t.Errorf("expected 1 CSR approved, got %d", result.CSRsApproved)
	}
	if len(result.CSRNames) != 1 || result.CSRNames[0] != "csr-test1" {
		t.Errorf("expected CSR name csr-test1, got %v", result.CSRNames)
	}
}
