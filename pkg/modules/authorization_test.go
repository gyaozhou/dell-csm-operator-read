// Copyright (c) 2022-2024 Dell Inc., or its subsidiaries. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//  http://www.apache.org/licenses/LICENSE-2.0

package modules

import (
	"context"
	"fmt"
	"strings"
	"testing"

	csmv1 "github.com/dell/csm-operator/api/v1"
	drivers "github.com/dell/csm-operator/pkg/drivers"
	utils "github.com/dell/csm-operator/pkg/utils"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	applyv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	acorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlClientFake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCheckAnnotationAuth(t *testing.T) {
	t.Run("it handles an empty annotation", func(t *testing.T) {
		var empty map[string]string
		err := CheckAnnotationAuth(empty)
		if err == nil {
			t.Errorf("expected non-nil err, got %v", err)
		}
	})

	t.Run("it handles an incorrect auth annotation", func(t *testing.T) {
		want := "com.dell.karavi-authorization-proxy"
		invalid := map[string]string{
			"annotation": "test.proxy",
		}
		got := CheckAnnotationAuth(invalid)
		if got == nil {
			t.Errorf("got %v, expected annotation to be %s", got, want)
		}
	})

	t.Run("it handles an invalid annotation", func(t *testing.T) {
		got := map[string]string{
			"com.dell.karavi-authorization-proxy": "false",
		}
		err := CheckAnnotationAuth(got)
		if err == nil {
			t.Errorf("got %v, expected annotation to be true %s", got, err)
		}
	})
}

func TestCheckApplyVolumesAuth(t *testing.T) {
	got := []acorev1.VolumeApplyConfiguration{}
	err := CheckApplyVolumesAuth(got)
	if err == nil {
		t.Errorf("got %v, expected karavi-authorization-config volume", got)
	}
}

func TestCheckApplyContainersAuth(t *testing.T) {
	t.Run("it handles no volume mount", func(t *testing.T) {
		got := []acorev1.ContainerApplyConfiguration{
			*acorev1.Container().WithName("karavi-authorization-proxy"),
		}
		driver := "powerscale"
		err := CheckApplyContainersAuth(got, driver, true)
		if err == nil {
			t.Errorf("got %v, expected karavi-authorization-config to be injected", got)
		}
	})

	t.Run("it handles an empty container", func(t *testing.T) {
		got := []acorev1.ContainerApplyConfiguration{}
		driver := "powerscale"
		err := CheckApplyContainersAuth(got, driver, true)
		if err == nil {
			t.Errorf("got %v, expected karavi-authorization-config to be injected", got)
		}
	})
}

func TestAuthInjectDaemonset(t *testing.T) {
	ctx := context.Background()
	correctlyInjected := func(ds applyv1.DaemonSetApplyConfiguration, drivertype string, skipCertificateValidation bool) error {
		err := CheckAnnotationAuth(ds.Annotations)
		if err != nil {
			return err
		}
		err = CheckApplyVolumesAuth(ds.Spec.Template.Spec.Volumes)
		if err != nil {
			return err
		}

		err = CheckApplyContainersAuth(ds.Spec.Template.Spec.Containers, drivertype, skipCertificateValidation)
		if err != nil {
			return err
		}
		return nil
	}
	//*appsv1.DaemonSet
	tests := map[string]func(t *testing.T) (bool, bool, applyv1.DaemonSetApplyConfiguration, utils.OperatorConfig){
		"success - greenfield injection": func(*testing.T) (bool, bool, applyv1.DaemonSetApplyConfiguration, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			nodeYAML, err := drivers.GetNode(ctx, customResource, operatorConfig, csmv1.PowerScaleName, "node.yaml")
			if err != nil {
				panic(err)
			}

			return true, true, nodeYAML.DaemonSetApplyConfig, operatorConfig
		},
		"success - brownfield injection": func(*testing.T) (bool, bool, applyv1.DaemonSetApplyConfiguration, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			nodeYAML, err := drivers.GetNode(ctx, customResource, operatorConfig, csmv1.PowerScaleName, "node.yaml")
			if err != nil {
				panic(err)
			}
			newDaemonSet, err := AuthInjectDaemonset(nodeYAML.DaemonSetApplyConfig, customResource, operatorConfig)
			if err != nil {
				panic(err)
			}

			return true, true, *newDaemonSet, operatorConfig
		},
		"success - validate certificate": func(*testing.T) (bool, bool, applyv1.DaemonSetApplyConfiguration, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth_validate_cert.yaml")
			if err != nil {
				panic(err)
			}
			nodeYAML, err := drivers.GetNode(ctx, customResource, operatorConfig, csmv1.PowerScaleName, "node.yaml")
			if err != nil {
				panic(err)
			}
			newDaemonSet, err := AuthInjectDaemonset(nodeYAML.DaemonSetApplyConfig, customResource, operatorConfig)
			if err != nil {
				panic(err)
			}

			return true, false, *newDaemonSet, operatorConfig
		},
		"fail - bad config path": func(*testing.T) (bool, bool, applyv1.DaemonSetApplyConfiguration, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			nodeYAML, err := drivers.GetNode(ctx, customResource, operatorConfig, csmv1.PowerScaleName, "node.yaml")
			if err != nil {
				panic(err)
			}
			tmpOperatorConfig := operatorConfig
			tmpOperatorConfig.ConfigDirectory = "bad/path"

			return false, false, nodeYAML.DaemonSetApplyConfig, tmpOperatorConfig
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			success, skipCertificateValidation, ds, opConfig := tc(t)
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			newDaemonSet, err := AuthInjectDaemonset(ds, customResource, opConfig)
			if success {
				assert.NoError(t, err)
				if err := correctlyInjected(*newDaemonSet, string(customResource.Spec.Driver.CSIDriverType), skipCertificateValidation); err != nil {
					assert.NoError(t, err)
				}
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestAuthInjectDeployment(t *testing.T) {
	ctx := context.Background()
	correctlyInjected := func(dp applyv1.DeploymentApplyConfiguration, drivertype string, skipCertificateValidation bool) error {
		err := CheckAnnotationAuth(dp.Annotations)
		if err != nil {
			return err
		}
		err = CheckApplyVolumesAuth(dp.Spec.Template.Spec.Volumes)
		if err != nil {
			return err
		}
		err = CheckApplyContainersAuth(dp.Spec.Template.Spec.Containers, drivertype, skipCertificateValidation)
		if err != nil {
			return err
		}
		return nil
	}

	tests := map[string]func(t *testing.T) (bool, bool, applyv1.DeploymentApplyConfiguration, utils.OperatorConfig, csmv1.ContainerStorageModule){
		"success - greenfield injection": func(*testing.T) (bool, bool, applyv1.DeploymentApplyConfiguration, utils.OperatorConfig, csmv1.ContainerStorageModule) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			controllerYAML, err := drivers.GetController(ctx, customResource, operatorConfig, csmv1.PowerScaleName)
			if err != nil {
				panic(err)
			}
			return true, true, controllerYAML.Deployment, operatorConfig, customResource
		},
		"success - brownfield injection": func(*testing.T) (bool, bool, applyv1.DeploymentApplyConfiguration, utils.OperatorConfig, csmv1.ContainerStorageModule) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			tmpCR := customResource
			controllerYAML, err := drivers.GetController(ctx, tmpCR, operatorConfig, csmv1.PowerScaleName)
			if err != nil {
				panic(err)
			}
			newDeployment, err := AuthInjectDeployment(controllerYAML.Deployment, tmpCR, operatorConfig)
			if err != nil {
				panic(err)
			}

			return true, true, *newDeployment, operatorConfig, customResource
		},
		"success - validate certificate": func(*testing.T) (bool, bool, applyv1.DeploymentApplyConfiguration, utils.OperatorConfig, csmv1.ContainerStorageModule) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth_validate_cert.yaml")
			if err != nil {
				panic(err)
			}
			tmpCR := customResource
			controllerYAML, err := drivers.GetController(ctx, tmpCR, operatorConfig, csmv1.PowerScaleName)
			if err != nil {
				panic(err)
			}
			newDeployment, err := AuthInjectDeployment(controllerYAML.Deployment, tmpCR, operatorConfig)
			if err != nil {
				panic(err)
			}

			return true, false, *newDeployment, operatorConfig, customResource
		},
		"fail - bad config path": func(*testing.T) (bool, bool, applyv1.DeploymentApplyConfiguration, utils.OperatorConfig, csmv1.ContainerStorageModule) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			controllerYAML, err := drivers.GetController(ctx, customResource, operatorConfig, csmv1.PowerScaleName)
			if err != nil {
				panic(err)
			}
			tmpOperatorConfig := operatorConfig
			tmpOperatorConfig.ConfigDirectory = "bad/path"

			return false, true, controllerYAML.Deployment, tmpOperatorConfig, customResource
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			success, skipCertificateValidation, dp, opConfig, cr := tc(t)
			newDeployment, err := AuthInjectDeployment(dp, cr, opConfig)
			if success {
				assert.NoError(t, err)
				if err := correctlyInjected(*newDeployment, string(cr.Spec.Driver.CSIDriverType), skipCertificateValidation); err != nil {
					assert.NoError(t, err)
				}
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestAuthorizationPreCheck(t *testing.T) {
	tests := map[string]func(t *testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client){
		"success": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			namespace := customResource.Namespace
			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]

			karaviAuthconfig := getSecret(namespace, "karavi-authorization-config")
			proxyAuthzTokens := getSecret(namespace, "proxy-authz-tokens")

			client := ctrlClientFake.NewClientBuilder().WithObjects(karaviAuthconfig, proxyAuthzTokens).Build()

			return true, auth, tmpCR, client
		},
		"success - version provided": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			namespace := customResource.Namespace
			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]
			auth.ConfigVersion = "v2.0.0-alpha"

			karaviAuthconfig := getSecret(namespace, "karavi-authorization-config")
			proxyAuthzTokens := getSecret(namespace, "proxy-authz-tokens")

			client := ctrlClientFake.NewClientBuilder().WithObjects(karaviAuthconfig, proxyAuthzTokens).Build()

			return true, auth, tmpCR, client
		},
		"fail - SKIP_CERTIFICATE_VALIDATION is false but no cert": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			namespace := customResource.Namespace
			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]

			// set skipCertificateValidation to false
			for i, env := range auth.Components[0].Envs {
				if env.Name == "SKIP_CERTIFICATE_VALIDATION" {
					auth.Components[0].Envs[i].Value = "false"
				}
			}

			karaviAuthconfig := getSecret(namespace, "karavi-authorization-config")
			proxyAuthzTokens := getSecret(namespace, "proxy-authz-tokens")
			client := ctrlClientFake.NewClientBuilder().WithObjects(karaviAuthconfig, proxyAuthzTokens).Build()

			return false, auth, tmpCR, client
		},
		"fail - invalid SKIP_CERTIFICATE_VALIDATION value": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]
			// set skipCertificateValidation to false
			for i, env := range auth.Components[0].Envs {
				if env.Name == "SKIP_CERTIFICATE_VALIDATION" {
					auth.Components[0].Envs[i].Value = "1234"
				}
			}

			client := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, auth, tmpCR, client
		},
		"fail - empty proxy host": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}
			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]

			for i, env := range auth.Components[0].Envs {
				if env.Name == "PROXY_HOST" {
					auth.Components[0].Envs[i].Value = ""
				}
			}
			client := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, auth, tmpCR, client
		},

		"fail - unsupported driver": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			tmpCR.Spec.Driver.CSIDriverType = "unsupported-driver"
			auth := tmpCR.Spec.Modules[0]

			client := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, auth, tmpCR, client
		},
		"fail - unsupported auth version": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_auth.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]
			auth.ConfigVersion = "v100000.0.0"

			client := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, auth, tmpCR, client
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			success, auth, tmpCR, client := tc(t)
			err := AuthorizationPrecheck(context.TODO(), operatorConfig, auth, tmpCR, client)
			if success {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestAuthorizationServerPreCheck(t *testing.T) {
	type fakeControllerRuntimeClientWrapper func(clusterConfigData []byte) (ctrlClient.Client, error)

	tests := map[string]func(t *testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client, fakeControllerRuntimeClientWrapper){
		"success v1": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client, fakeControllerRuntimeClientWrapper) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_v1100.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]

			karaviConfig := getSecret(customResource.Namespace, "karavi-config-secret")
			karaviStorage := getSecret(customResource.Namespace, "karavi-storage-secret")
			karaviTLS := getSecret(customResource.Namespace, "karavi-selfsigned-tls")

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviConfig, karaviStorage, karaviTLS).Build()

			fakeControllerRuntimeClient := func(_ []byte) (ctrlClient.Client, error) {
				clusterClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviConfig, karaviStorage, karaviTLS).Build()
				return clusterClient, nil
			}

			return true, auth, tmpCR, sourceClient, fakeControllerRuntimeClient
		},
		"success v2": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client, fakeControllerRuntimeClientWrapper) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]

			karaviConfig := getSecret(customResource.Namespace, "karavi-config-secret")
			karaviTLS := getSecret(customResource.Namespace, "karavi-selfsigned-tls")

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviConfig, karaviTLS).Build()

			fakeControllerRuntimeClient := func(_ []byte) (ctrlClient.Client, error) {
				clusterClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviConfig, karaviTLS).Build()
				return clusterClient, nil
			}

			return true, auth, tmpCR, sourceClient, fakeControllerRuntimeClient
		},
		"success - version provided": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client, fakeControllerRuntimeClientWrapper) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]
			auth.ConfigVersion = "v2.0.0-alpha"
			karaviConfig := getSecret(customResource.Namespace, "karavi-config-secret")
			karaviStorage := getSecret(customResource.Namespace, "karavi-storage-secret")
			karaviTLS := getSecret(customResource.Namespace, "karavi-selfsigned-tls")

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviConfig, karaviStorage, karaviTLS).Build()
			fakeControllerRuntimeClient := func(_ []byte) (ctrlClient.Client, error) {
				clusterClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviConfig, karaviStorage, karaviTLS).Build()
				return clusterClient, nil
			}

			return true, auth, tmpCR, sourceClient, fakeControllerRuntimeClient
		},
		"fail - unsupported authorization version": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client, fakeControllerRuntimeClientWrapper) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]
			auth.ConfigVersion = "v100000.0.0"

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			fakeControllerRuntimeClient := func(_ []byte) (ctrlClient.Client, error) {
				return ctrlClientFake.NewClientBuilder().WithObjects().Build(), nil
			}

			return false, auth, tmpCR, sourceClient, fakeControllerRuntimeClient
		},
		"fail v1 - karavi-config-secret not found": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client, fakeControllerRuntimeClientWrapper) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_v1100.yaml")
			if err != nil {
				panic(err)
			}
			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]

			karaviStorage := getSecret(customResource.Namespace, "karavi-storage-secret")
			karaviTLS := getSecret(customResource.Namespace, "karavi-selfsigned-tls")
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviStorage, karaviTLS).Build()

			fakeControllerRuntimeClient := func(_ []byte) (ctrlClient.Client, error) {
				clusterClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviStorage, karaviTLS).Build()
				return clusterClient, nil
			}

			return false, auth, tmpCR, sourceClient, fakeControllerRuntimeClient
		},
		"fail v1 - karavi-storage-secret not found": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client, fakeControllerRuntimeClientWrapper) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_v1100.yaml")
			if err != nil {
				panic(err)
			}
			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]

			karaviConfig := getSecret(customResource.Namespace, "karavi-config-secret")
			karaviTLS := getSecret(customResource.Namespace, "karavi-selfsigned-tls")
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviConfig, karaviTLS).Build()

			fakeControllerRuntimeClient := func(_ []byte) (ctrlClient.Client, error) {
				clusterClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviConfig, karaviTLS).Build()
				return clusterClient, nil
			}

			return false, auth, tmpCR, sourceClient, fakeControllerRuntimeClient
		},
		"fail v2 - karavi-config-secret not found": func(*testing.T) (bool, csmv1.Module, csmv1.ContainerStorageModule, ctrlClient.Client, fakeControllerRuntimeClientWrapper) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}
			tmpCR := customResource
			auth := tmpCR.Spec.Modules[0]

			karaviTLS := getSecret(customResource.Namespace, "karavi-selfsigned-tls")
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviTLS).Build()

			fakeControllerRuntimeClient := func(_ []byte) (ctrlClient.Client, error) {
				clusterClient := ctrlClientFake.NewClientBuilder().WithObjects(karaviTLS).Build()
				return clusterClient, nil
			}

			return false, auth, tmpCR, sourceClient, fakeControllerRuntimeClient
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			oldNewControllerRuntimeClientWrapper := utils.NewControllerRuntimeClientWrapper
			oldNewK8sClientWrapper := utils.NewK8sClientWrapper
			defer func() {
				utils.NewControllerRuntimeClientWrapper = oldNewControllerRuntimeClientWrapper
				utils.NewK8sClientWrapper = oldNewK8sClientWrapper
			}()

			success, auth, tmpCR, sourceClient, fakeControllerRuntimeClient := tc(t)
			utils.NewControllerRuntimeClientWrapper = fakeControllerRuntimeClient
			utils.NewK8sClientWrapper = func(_ []byte) (*kubernetes.Clientset, error) {
				return nil, nil
			}

			fakeReconcile := utils.FakeReconcileCSM{
				Client:    sourceClient,
				K8sClient: fake.NewSimpleClientset(),
			}

			err := AuthorizationServerPrecheck(context.TODO(), operatorConfig, auth, tmpCR, &fakeReconcile)
			if success {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestAuthorizationServerDeployment(t *testing.T) {
	tests := map[string]func(t *testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig){
		"success - deleting": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			cm := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind: "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "csm-config-params",
				},
			}

			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(cm).Build()

			return true, true, tmpCR, sourceClient, operatorConfig
		},
		"success - creating": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return true, false, tmpCR, sourceClient, operatorConfig
		},
		"success - creating with vault client certificates": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_vault_cert.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return true, false, tmpCR, sourceClient, operatorConfig
		},
		"success - creating v1": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_v1100.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return true, false, tmpCR, sourceClient, operatorConfig
		},
		"success - creating with default redis storage class": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_no_redis.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return true, false, tmpCR, sourceClient, operatorConfig
		},
		"fail - authorization module not found": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_replica.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, false, tmpCR, sourceClient, operatorConfig
		},
		"fail - corrupt vault ca": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_bad_vault_ca.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, false, tmpCR, sourceClient, operatorConfig
		},
		"fail - corrupt vault client cert": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_bad_vault_cert.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, false, tmpCR, sourceClient, operatorConfig
		},
		"fail - corrupt vault client key": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_bad_vault_key.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, false, tmpCR, sourceClient, operatorConfig
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			success, isDeleting, cr, sourceClient, op := tc(t)

			err := AuthorizationServerDeployment(context.TODO(), isDeleting, op, cr, sourceClient)
			if success {
				assert.NoError(t, err)
			} else {
				fmt.Println(err)
				assert.Error(t, err)
			}
		})
	}
}

func TestAuthorizationKubeMgmtPolicies(t *testing.T) {
	cr, err := getCustomResource("./testdata/cr_auth_proxy_diff_namespace.yaml")
	if err != nil {
		t.Fatal(err)
	}

	certmanagerv1.AddToScheme(scheme.Scheme)
	sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

	err = AuthorizationServerDeployment(context.TODO(), false, operatorConfig, cr, sourceClient)
	if err != nil {
		t.Fatal(err)
	}

	proxyServer := &appsv1.Deployment{}
	err = sourceClient.Get(context.Background(), types.NamespacedName{Name: "proxy-server", Namespace: "dell"}, proxyServer)
	if err != nil {
		t.Fatal(err)
	}

	argFound := false
	for _, container := range proxyServer.Spec.Template.Spec.Containers {
		if container.Name == "kube-mgmt" {
			for _, arg := range container.Args {
				if strings.Contains(arg, "--policies") {
					argFound = true
					if arg != "--policies=dell" {
						t.Fatalf("expected --policies=dell, got %s", arg)
					}
					break
				}
			}
		}
		if argFound {
			break
		}
	}

	if !argFound {
		t.Fatalf("expected --policies=dell, got none")
	}
}

func TestAuthorizationIngress(t *testing.T) {
	tests := map[string]func(t *testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client){
		"success - deleting": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			i1 := &networking.Ingress{
				TypeMeta: metav1.TypeMeta{
					Kind: "Ingress",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "proxy-server",
				},
			}

			i2 := &networking.Ingress{
				TypeMeta: metav1.TypeMeta{
					Kind: "Ingress",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "tenant-service",
				},
			}

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(i1, i2).Build()

			return true, true, tmpCR, sourceClient
		},
		"success - creating": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			namespace := customResource.Namespace
			name := namespace + "-ingress-nginx-controller"

			dp := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind: "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app.kubernetes.io/name": "ingress-nginx"},
					},
				},
			}

			pod := &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind: "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			}

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(dp, pod).Build()

			return true, true, tmpCR, sourceClient
		},
		"success - creating with certs": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_certs.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			namespace := customResource.Namespace
			name := namespace + "-ingress-nginx-controller"

			dp := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind: "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app.kubernetes.io/name": "ingress-nginx"},
					},
				},
			}

			pod := &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind: "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			}

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(dp, pod).Build()

			return true, true, tmpCR, sourceClient
		},
		"success - creating with openshift and other annotations": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_openshift.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return true, true, tmpCR, sourceClient
		},
		"success - creating v1.10.0": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_v1100.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			namespace := customResource.Namespace
			name := namespace + "-ingress-nginx-controller"

			dp := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind: "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app.kubernetes.io/name": "ingress-nginx"},
					},
				},
			}

			pod := &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind: "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			}

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(dp, pod).Build()

			return true, true, tmpCR, sourceClient
		},
		"success - creating v1.9.0": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_v190.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			namespace := customResource.Namespace
			name := namespace + "-ingress-nginx-controller"

			dp := &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{
					Kind: "Deployment",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: appsv1.DeploymentSpec{
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app.kubernetes.io/name": "ingress-nginx"},
					},
				},
			}

			pod := &corev1.Pod{
				TypeMeta: metav1.TypeMeta{
					Kind: "Pod",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
			}

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(dp, pod).Build()

			return true, true, tmpCR, sourceClient
		},
		"fail - wrong module name": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_replica.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, false, tmpCR, sourceClient
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			success, isDeleting, cr, sourceClient := tc(t)
			fakeReconcile := utils.FakeReconcileCSM{
				Client:    sourceClient,
				K8sClient: fake.NewSimpleClientset(),
			}
			err := AuthorizationIngress(context.TODO(), isDeleting, true, cr, &fakeReconcile, sourceClient)
			if success {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestInstallPolicies(t *testing.T) {
	tests := map[string]func(t *testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig){
		"success - deleting": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			cr := &corev1.ConfigMap{
				TypeMeta: metav1.TypeMeta{
					Kind: "ConfigMap",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "common",
				},
			}

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(cr).Build()

			return true, true, tmpCR, sourceClient, operatorConfig
		},
		"success - creating": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return true, false, tmpCR, sourceClient, operatorConfig
		},

		"fail - wrong module name": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_replica.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, false, tmpCR, sourceClient, operatorConfig
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			success, isDeleting, cr, sourceClient, op := tc(t)

			err := InstallPolicies(context.TODO(), isDeleting, op, cr, sourceClient)
			if success {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestNginxIngressController(t *testing.T) {
	tests := map[string]func(t *testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig){
		"success - deleting": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			cr := &networking.IngressClass{
				TypeMeta: metav1.TypeMeta{
					Kind: "IngressClass",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "nginx",
				},
			}

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(cr).Build()
			return true, true, tmpCR, sourceClient, operatorConfig
		},
		"success - creating": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()
			return true, false, tmpCR, sourceClient, operatorConfig
		},

		"fail - wrong module name": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_powerscale_replica.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, false, tmpCR, sourceClient, operatorConfig
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			success, isDeleting, cr, sourceClient, op := tc(t)

			err := NginxIngressController(context.TODO(), isDeleting, op, cr, sourceClient)
			if success {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestAuthorizationCertificates(t *testing.T) {
	tests := map[string]func(t *testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig){
		"success - using self-signed certificate": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return true, true, tmpCR, sourceClient, operatorConfig
		},
		"success - using custom tls secret": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_certs.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return true, false, tmpCR, sourceClient, operatorConfig
		},

		"fail - using partial custom cert": func(*testing.T) (bool, bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_certs_missing_key.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			certmanagerv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, false, tmpCR, sourceClient, operatorConfig
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			success, isDeleting, cr, sourceClient, op := tc(t)

			err := InstallWithCerts(context.TODO(), isDeleting, op, cr, sourceClient)
			if success {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestAuthorizationCrdDeploy(t *testing.T) {
	tests := map[string]func(t *testing.T) (bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig){
		"success - deleting": func(*testing.T) (bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			cr := &apiextv1.CustomResourceDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind: "CustomResourceDefinition",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "csmroles.csm-authorization.storage.dell.com",
				},
			}
			apiextv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects(cr).Build()
			return true, tmpCR, sourceClient, operatorConfig
		},
		"success - creating": func(*testing.T) (bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			apiextv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()
			return true, tmpCR, sourceClient, operatorConfig
		},
		"success - creating v1": func(*testing.T) (bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy_v1100.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource

			apiextv1.AddToScheme(scheme.Scheme)
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()
			return true, tmpCR, sourceClient, operatorConfig
		},
		"fail - auth deployment file bad yaml": func(*testing.T) (bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			badOperatorConfig.ConfigDirectory = "./testdata/badYaml"
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, tmpCR, sourceClient, badOperatorConfig
		},
		"fail - auth config file not found": func(*testing.T) (bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_auth_proxy.yaml")
			if err != nil {
				panic(err)
			}

			tmpCR := customResource
			badOperatorConfig.ConfigDirectory = "invalid-dir"
			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()

			return false, tmpCR, sourceClient, badOperatorConfig
		},
		"fail - auth module not found": func(*testing.T) (bool, csmv1.ContainerStorageModule, ctrlClient.Client, utils.OperatorConfig) {
			customResource, err := getCustomResource("./testdata/cr_application_mobility.yaml")
			if err != nil {
				panic(err)
			}

			sourceClient := ctrlClientFake.NewClientBuilder().WithObjects().Build()
			tmpCR := customResource

			return false, tmpCR, sourceClient, operatorConfig
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			success, cr, sourceClient, op := tc(t)

			err := AuthCrdDeploy(ctx, op, cr, sourceClient)
			if success {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
