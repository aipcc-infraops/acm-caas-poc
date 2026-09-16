package client

import "k8s.io/apimachinery/pkg/runtime/schema"

var (
	GVRClusterDeployment = schema.GroupVersionResource{
		Group: "hive.openshift.io", Version: "v1", Resource: "clusterdeployments",
	}
	GVRClusterImageSet = schema.GroupVersionResource{
		Group: "hive.openshift.io", Version: "v1", Resource: "clusterimagesets",
	}
	GVRManagedCluster = schema.GroupVersionResource{
		Group: "cluster.open-cluster-management.io", Version: "v1", Resource: "managedclusters",
	}
	GVRManagedClusterInfo = schema.GroupVersionResource{
		Group: "internal.open-cluster-management.io", Version: "v1beta1", Resource: "managedclusterinfos",
	}
	GVRManifestWork = schema.GroupVersionResource{
		Group: "work.open-cluster-management.io", Version: "v1", Resource: "manifestworks",
	}
	GVRPolicy = schema.GroupVersionResource{
		Group: "policy.open-cluster-management.io", Version: "v1", Resource: "policies",
	}
	GVRPlacementBinding = schema.GroupVersionResource{
		Group: "policy.open-cluster-management.io", Version: "v1", Resource: "placementbindings",
	}
	GVRPlacementRule = schema.GroupVersionResource{
		Group: "apps.open-cluster-management.io", Version: "v1", Resource: "placementrules",
	}
	GVRPlacement = schema.GroupVersionResource{
		Group: "cluster.open-cluster-management.io", Version: "v1beta1", Resource: "placements",
	}
	GVRKlusterletAddonConfig = schema.GroupVersionResource{
		Group: "agent.open-cluster-management.io", Version: "v1", Resource: "klusterletaddonconfigs",
	}
	GVRNamespace = schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "namespaces",
	}
	GVRSecret = schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "secrets",
	}
	GVRMultiClusterObservability = schema.GroupVersionResource{
		Group: "observability.open-cluster-management.io", Version: "v1beta2", Resource: "multiclusterobservabilities",
	}
	GVRDeployment = schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}
	GVRService = schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "services",
	}
	GVRPersistentVolumeClaim = schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "persistentvolumeclaims",
	}
	GVRConfigurationPolicy = schema.GroupVersionResource{
		Group: "policy.open-cluster-management.io", Version: "v1", Resource: "configurationpolicies",
	}
	GVRManagedClusterSet = schema.GroupVersionResource{
		Group: "cluster.open-cluster-management.io", Version: "v1beta2", Resource: "managedclustersets",
	}
	GVRManagedClusterSetBinding = schema.GroupVersionResource{
		Group: "cluster.open-cluster-management.io", Version: "v1beta2", Resource: "managedclustersetbindings",
	}
	GVRManagedClusterImageRegistry = schema.GroupVersionResource{
		Group: "imageregistry.open-cluster-management.io", Version: "v1alpha1", Resource: "managedclusterimageregistries",
	}
	GVRMachinePool = schema.GroupVersionResource{
		Group: "hive.openshift.io", Version: "v1", Resource: "machinepools",
	}
	GVRConfigMap = schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "configmaps",
	}
	GVRClusterCurator = schema.GroupVersionResource{
		Group: "cluster.open-cluster-management.io", Version: "v1beta1", Resource: "clustercurators",
	}
	GVROperatorPolicy = schema.GroupVersionResource{
		Group: "policy.open-cluster-management.io", Version: "v1beta1", Resource: "operatorpolicies",
	}
	GVRCertificatePolicy = schema.GroupVersionResource{
		Group: "policy.open-cluster-management.io", Version: "v1", Resource: "certificatepolicies",
	}
	GVRClusterPool = schema.GroupVersionResource{
		Group: "hive.openshift.io", Version: "v1", Resource: "clusterpools",
	}
	GVRClusterClaim = schema.GroupVersionResource{
		Group: "hive.openshift.io", Version: "v1", Resource: "clusterclaims",
	}
	GVRManifestWorkReplicaSet = schema.GroupVersionResource{
		Group: "work.open-cluster-management.io", Version: "v1alpha1", Resource: "manifestworkreplicasets",
	}
	GVRManagedServiceAccount = schema.GroupVersionResource{
		Group: "authentication.open-cluster-management.io", Version: "v1beta1", Resource: "managedserviceaccounts",
	}
	GVRManagedClusterAddOn = schema.GroupVersionResource{
		Group: "addon.open-cluster-management.io", Version: "v1alpha1", Resource: "managedclusteraddons",
	}
)
