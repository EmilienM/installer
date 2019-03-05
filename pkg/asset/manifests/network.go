package manifests

import (
	"fmt"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/installer/pkg/asset"
	"github.com/openshift/installer/pkg/asset/installconfig"
	"github.com/openshift/installer/pkg/asset/templates/content/openshift"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1a1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

var (
	noCrdFilename = filepath.Join(manifestDir, "cluster-network-01-crd.yml")
	noCfgFilename = filepath.Join(manifestDir, "cluster-network-02-config.yml")
)

// We need to manually create our CRDs first, so we can create the
// configuration instance of it in the installer. Other operators have
// their CRD created by the CVO, but we need to create the corresponding
// CRs in the installer, so we need the CRD to be there.
// The first CRD is the high-level Network.config.openshift.io object,
// which is stable ahd minimal. Administrators can override configure the
// network in a more detailed manner with the operator-specific CR, which
// also needs to be done before the installer is run, so we provide both.

// Networking generates the cluster-network-*.yml files.
type Networking struct {
	Config   *configv1.Network
	FileList []*asset.File
}

var _ asset.WritableAsset = (*Networking)(nil)

// Name returns a human friendly name for the operator.
func (no *Networking) Name() string {
	return "Network Config"
}

// Dependencies returns all of the dependencies directly needed to generate
// network configuration.
func (no *Networking) Dependencies() []asset.Asset {
	return []asset.Asset{
		&installconfig.InstallConfig{},
		&openshift.NetworkCRDs{},
	}
}

// Generate generates the network operator config and its CRD.
func (no *Networking) Generate(dependencies asset.Parents) error {
	installConfig := &installconfig.InstallConfig{}
	crds := &openshift.NetworkCRDs{}
	dependencies.Get(installConfig, crds)

	netConfig := installConfig.Config.Networking

	clusterNet := []configv1.ClusterNetworkEntry{}
	if len(netConfig.ClusterNetwork) > 0 {
		for _, net := range netConfig.ClusterNetwork {
			clusterNet = append(clusterNet, configv1.ClusterNetworkEntry{
				CIDR:       net.CIDR.String(),
				HostPrefix: uint32(net.HostPrefix),
			})
		}
	} else {
		return errors.Errorf("ClusterNetworks must be specified")
	}

	serviceNet := []string{}
	for _, sn := range netConfig.ServiceNetwork {
		serviceNet = append(serviceNet, sn.String())
	}

	no.Config = &configv1.Network{
		TypeMeta: metav1.TypeMeta{
			APIVersion: configv1.SchemeGroupVersion.String(),
			Kind:       "Network",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
			// not namespaced
		},
		Spec: configv1.NetworkSpec{
			ClusterNetwork: clusterNet,
			ServiceNetwork: serviceNet,
			NetworkType:    netConfig.NetworkType,
		},
	}

	configData, err := yaml.Marshal(no.Config)
	if err != nil {
		return errors.Wrapf(err, "failed to create %s manifests from InstallConfig", no.Name())
	}

	crdContents := ""
	for _, crdFile := range crds.Files() {
		crdContents = fmt.Sprintf("%s\n---\n%s", crdContents, crdFile.Data)
	}

	no.FileList = []*asset.File{
		{
			Filename: noCrdFilename,
			Data:     []byte(crdContents),
		},
		{
			Filename: noCfgFilename,
			Data:     configData,
		},
	}

	return nil
}

// Files returns the files generated by the asset.
func (no *Networking) Files() []*asset.File {
	return no.FileList
}

// ClusterNetwork returns the ClusterNetworkingConfig for the ClusterConfig
// object. This is called by ClusterK8sIO, which captures generalized cluster
// state but shouldn't need to be fully networking aware.
func (no *Networking) ClusterNetwork() (*clusterv1a1.ClusterNetworkingConfig, error) {
	if no.Config == nil {
		// should be unreachable.
		return nil, errors.Errorf("ClusterNetwork called before initialization")
	}

	pods := []string{}
	for _, cn := range no.Config.Spec.ClusterNetwork {
		pods = append(pods, cn.CIDR)
	}

	cn := &clusterv1a1.ClusterNetworkingConfig{
		Services: clusterv1a1.NetworkRanges{
			CIDRBlocks: no.Config.Spec.ServiceNetwork,
		},
		Pods: clusterv1a1.NetworkRanges{
			CIDRBlocks: pods,
		},
	}
	return cn, nil
}

// Load returns false since this asset is not written to disk by the installer.
func (no *Networking) Load(f asset.FileFetcher) (bool, error) {
	return false, nil
}
