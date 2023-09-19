/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/gateway-api/conformance/utils/kubernetes"
	"sigs.k8s.io/gateway-api/conformance/utils/suite"
)

func init() {
	ConformanceTests = append(ConformanceTests, GatewayStaticAddresses)
}

var GatewayStaticAddresses = suite.ConformanceTest{
	ShortName:   "GatewayStaticAddresses",
	Description: "A Gateway in the gateway-conformance-infra namespace should be able to use a previously determined addresses.",
	Features: []suite.SupportedFeature{
		suite.SupportGateway,
		suite.SupportGatewayStaticAddresses,
	},
	Manifests: []string{
		"tests/gateway-static-addresses.yaml",
	},
	Test: func(t *testing.T, s *suite.ConformanceTestSuite) {
		gwNN := types.NamespacedName{
			Name:      "gateway-static-addresses",
			Namespace: "gateway-conformance-infra",
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		t.Logf("waiting for namespace %s and Gateway %s to be ready for testing", gwNN.Namespace, gwNN.Name)
		namespaces := []string{gwNN.Namespace}
		kubernetes.NamespacesMustBeReady(t, s.Client, s.TimeoutConfig, namespaces)
		kubernetes.GatewayMustHaveLatestConditions(t, s.Client, s.TimeoutConfig, gwNN)

		t.Logf("retrieving Gateway %s/%s and noting the provided addresses", gwNN.Namespace, gwNN.Name)
		currentGW := &v1beta1.Gateway{}
		err := s.Client.Get(ctx, gwNN, currentGW)
		require.NoError(t, err, "error getting Gateway: %v", err)
		require.Len(t, currentGW.Spec.Addresses, 3, "expected 3 addresses on the Gateway, one invalid, one usable and one unusable. somehow got %d", len(currentGW.Spec.Addresses))
		invalidAddress := currentGW.Spec.Addresses[0]
		unusableAddress := currentGW.Spec.Addresses[1]
		usableAddress := currentGW.Spec.Addresses[2]

		t.Logf("verifying that the Gateway %s/%s is NOT accepted due to an address type that the implementation doesn't support", gwNN.Namespace, gwNN.Name)
		kubernetes.GatewayAcceptedConditionMustEqual(t, currentGW, metav1.ConditionFalse, v1beta1.GatewayReasonUnsupportedAddress)

		t.Logf("patching Gateway %s/%s to remove the invalid address", gwNN.Namespace, gwNN.Name)
		updatedGW := currentGW.DeepCopy()
		updatedGW.Spec.Addresses = filterAddr(currentGW.Spec.Addresses, invalidAddress)
		err = s.Client.Patch(ctx, updatedGW, client.MergeFrom(currentGW))
		require.NoError(t, err, "failed to patch Gateway: %v", err)
		kubernetes.NamespacesMustBeReady(t, s.Client, s.TimeoutConfig, namespaces)
		kubernetes.GatewayMustHaveLatestConditions(t, s.Client, s.TimeoutConfig, gwNN)

		t.Logf("verifying that the Gateway %s/%s is now accepted, but is not programmed due to an address that can't be used", gwNN.Namespace, gwNN.Name)
		err = s.Client.Get(ctx, gwNN, currentGW)
		require.NoError(t, err, "error getting Gateway: %v", err)
		kubernetes.GatewayAcceptedConditionMustEqual(t, currentGW, metav1.ConditionTrue, v1beta1.GatewayReasonAccepted)
		kubernetes.GatewayProgrammedConditionMustEqual(t, currentGW, metav1.ConditionFalse, v1beta1.GatewayReasonAddressNotUsable)
		require.Len(t, currentGW.Status.Addresses, 1, "only one usable address was provided, so it should be the one reflected in status")
		require.Equal(t, usableAddress, currentGW.Status.Addresses[0], "expected usable address to be assigned")

		t.Logf("patching Gateway %s/%s to remove the unusable address", gwNN.Namespace, gwNN.Name)
		updatedGW = currentGW.DeepCopy()
		updatedGW.Spec.Addresses = filterAddr(currentGW.Spec.Addresses, unusableAddress)
		err = s.Client.Patch(ctx, updatedGW, client.MergeFrom(currentGW))
		require.NoError(t, err, "failed to patch Gateway: %v", err)
		kubernetes.NamespacesMustBeReady(t, s.Client, s.TimeoutConfig, namespaces)
		kubernetes.GatewayMustHaveLatestConditions(t, s.Client, s.TimeoutConfig, gwNN)

		t.Logf("verifying that the Gateway %s/%s is accepted and programmed with the usable static address assigned", gwNN.Namespace, gwNN.Name)
		err = s.Client.Get(ctx, gwNN, currentGW)
		require.NoError(t, err, "error getting Gateway: %v", err)
		kubernetes.GatewayAcceptedConditionMustEqual(t, currentGW, metav1.ConditionTrue, v1beta1.GatewayReasonAccepted)
		kubernetes.GatewayProgrammedConditionMustEqual(t, currentGW, metav1.ConditionTrue, v1beta1.GatewayReasonProgrammed)
		require.Len(t, currentGW.Spec.Addresses, 1, "expected only 1 address left specified on Gateway")
		require.Len(t, currentGW.Status.Addresses, 1, "only one usable address was provided, so it should be the one reflected in status")
		require.Equal(t, usableAddress, currentGW.Status.Addresses[0], "expected usable address to be assigned")
	},
}

// -----------------------------------------------------------------------------
// Private Helper Functions
// -----------------------------------------------------------------------------

func filterAddr(addrs []v1beta1.GatewayAddress, filter v1beta1.GatewayAddress) (newAddrs []v1beta1.GatewayAddress) {
	for _, addr := range addrs {
		if addr.Type != filter.Type || addr.Value != filter.Value {
			newAddrs = append(newAddrs, addr)
		}
	}
	return
}
