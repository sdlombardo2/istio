// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ra

import (
	"fmt"
	"time"

	cert "k8s.io/api/certificates/v1"
	clientset "k8s.io/client-go/kubernetes"

	"istio.io/istio/security/pkg/k8s/chiron"
	"istio.io/istio/security/pkg/pki/ca"
	raerror "istio.io/istio/security/pkg/pki/error"
	"istio.io/istio/security/pkg/pki/util"
)

// KubernetesRA integrated with an external CA using Kubernetes CSR API
type KubernetesRA struct {
	csrInterface  clientset.Interface
	keyCertBundle *util.KeyCertBundle
	raOpts        *IstioRAOptions
}

// NewKubernetesRA : Create a RA that interfaces with K8S CSR CA
func NewKubernetesRA(raOpts *IstioRAOptions) (*KubernetesRA, error) {
	keyCertBundle, err := util.NewKeyCertBundleWithRootCertFromFile(raOpts.CaCertFile)
	if err != nil {
		return nil, raerror.NewError(raerror.CAInitFail, fmt.Errorf("error processing Certificate Bundle for Kubernetes RA"))
	}
	istioRA := &KubernetesRA{
		csrInterface:  raOpts.K8sClient,
		raOpts:        raOpts,
		keyCertBundle: keyCertBundle,
	}
	return istioRA, nil
}

func (r *KubernetesRA) kubernetesSign(csrPEM []byte, caCertFile string, certSigner string,
	requestedLifetime time.Duration) ([]byte, error) {
	certSignerDomain := r.raOpts.CertSignerDomain
	if certSignerDomain == "" && certSigner != "" {
		return nil, raerror.NewError(raerror.CertGenError, fmt.Errorf("certSignerDomain is requiered for signer %s", certSigner))
	}
	if certSignerDomain != "" && certSigner != "" {
		certSigner = certSignerDomain + "/" + certSigner
	} else {
		certSigner = r.raOpts.CaSigner
	}
	usages := []cert.KeyUsage{
		cert.UsageDigitalSignature,
		cert.UsageKeyEncipherment,
		cert.UsageServerAuth,
		cert.UsageClientAuth,
	}
	certChain, _, err := chiron.SignCSRK8s(r.csrInterface, csrPEM, certSigner,
		nil, usages, "", caCertFile, true, false, requestedLifetime)
	if err != nil {
		return nil, raerror.NewError(raerror.CertGenError, err)
	}
	return certChain, err
}

// Sign takes a PEM-encoded CSR and cert opts, and returns a certificate signed by k8s CA.
func (r *KubernetesRA) Sign(csrPEM []byte, certOpts ca.CertOpts) ([]byte, error) {
	_, err := preSign(r.raOpts, csrPEM, certOpts.SubjectIDs, certOpts.TTL, certOpts.ForCA)
	if err != nil {
		return nil, err
	}
	certSigner := certOpts.CertSigner

	return r.kubernetesSign(csrPEM, r.raOpts.CaCertFile, certSigner, certOpts.TTL)
}

// SignWithCertChain is similar to Sign but returns the leaf cert and the entire cert chain.
func (r *KubernetesRA) SignWithCertChain(csrPEM []byte, certOpts ca.CertOpts) ([]byte, error) {
	cert, err := r.Sign(csrPEM, certOpts)
	if err != nil {
		return nil, err
	}
	chainPem := r.GetCAKeyCertBundle().GetCertChainPem()
	if len(chainPem) > 0 {
		cert = append(cert, chainPem...)
	}
	return cert, nil
}

// GetCAKeyCertBundle returns the KeyCertBundle for the CA.
func (r *KubernetesRA) GetCAKeyCertBundle() *util.KeyCertBundle {
	return r.keyCertBundle
}
