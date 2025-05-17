/*
Copyright 2025.

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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appv1 "github.com/anyflow/openapi-operator/api/v1"
)

// printObject prints the object in a readable JSON format
func printObject(obj interface{}, name string) {
	jsonBytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling %s: %v\n", name, err)
		return
	}
	fmt.Printf("\n=== %s ===\n%s\n", name, string(jsonBytes))
}

var _ = Describe("OpenapiService Controller", func() {
	const (
		OpenapiServiceName      = "test-openapi"
		OpenapiServiceNamespace = "default"
		timeout                = time.Second * 10
		interval              = time.Millisecond * 250
	)

	Context("When creating OpenapiService", func() {
		It("Should create WasmPlugin successfully", func() {
			By("Creating a new OpenapiService")
			ctx := context.Background()
			openapiService := &appv1.OpenapiService{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "operator.anyflow.net/v1",
					Kind:       "OpenapiService",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      OpenapiServiceName,
					Namespace: OpenapiServiceNamespace,
				},
				Spec: appv1.OpenapiServiceSpec{
					Selector: appv1.Selector{
						MatchLabels: map[string]string{
							"app": "test-app",
						},
					},
					Prefix: "test-prefix",
					OpenAPI: appv1.OpenAPISpec{
						Paths: map[string]struct{}{
							"/test/path": {},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, openapiService)).Should(Succeed())

			By("Checking if WasmPlugin is created")
			wasmPluginName := "path-template-filter-" + OpenapiServiceName
			Eventually(func() bool {
				var wasmPlugin unstructured.Unstructured
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      wasmPluginName,
					Namespace: OpenapiServiceNamespace,
				}, &wasmPlugin)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			By("Checking WasmPlugin spec")
			var wasmPlugin unstructured.Unstructured
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      wasmPluginName,
				Namespace: OpenapiServiceNamespace,
			}, &wasmPlugin)).Should(Succeed())

			spec := wasmPlugin.Object["spec"].(map[string]interface{})
			Expect(spec["url"]).Should(Equal("anyflow/path-template-filter:0.2.2"))
			Expect(spec["imagePullPolicy"]).Should(Equal("Always"))
			Expect(spec["phase"]).Should(Equal("STATS"))
			Expect(spec["failStrategy"]).Should(Equal("FAIL_OPEN"))
			Expect(spec["priority"]).Should(Equal(float64(10)))

			pluginConfig := spec["pluginConfig"].(map[string]interface{})
			Expect(pluginConfig["cacheSize"]).Should(Equal(float64(5)))

			services := pluginConfig["services"].([]interface{})
			service := services[0].(map[string]interface{})
			Expect(service["name"]).Should(Equal("test-prefix"))

			paths := service["paths"].(map[string]interface{})
			Expect(paths).Should(HaveKey("/test-prefix/test/path"))

			By("Checking OpenapiService status")
			Eventually(func() string {
				var updatedOpenapiService appv1.OpenapiService
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      OpenapiServiceName,
					Namespace: OpenapiServiceNamespace,
				}, &updatedOpenapiService)).Should(Succeed())
				return updatedOpenapiService.Status.WasmPluginName
			}, timeout, interval).Should(Equal(OpenapiServiceNamespace + "/" + wasmPluginName))
		})

		It("Should update WasmPlugin when OpenapiService is updated", func() {
			By("Updating OpenapiService")
			ctx := context.Background()
			var openapiService appv1.OpenapiService
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      OpenapiServiceName,
				Namespace: OpenapiServiceNamespace,
			}, &openapiService)).Should(Succeed())

			openapiService.Spec.Prefix = "updated-prefix"
			Expect(k8sClient.Update(ctx, &openapiService)).Should(Succeed())

			By("Checking if WasmPlugin is updated")
			var wasmPlugin unstructured.Unstructured
			Eventually(func() string {
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      "path-template-filter-" + OpenapiServiceName,
					Namespace: OpenapiServiceNamespace,
				}, &wasmPlugin)).Should(Succeed())
				spec := wasmPlugin.Object["spec"].(map[string]interface{})
				pluginConfig := spec["pluginConfig"].(map[string]interface{})
				services := pluginConfig["services"].([]interface{})
				service := services[0].(map[string]interface{})
				return service["name"].(string)
			}, timeout, interval).Should(Equal("updated-prefix"))
		})

		It("Should delete WasmPlugin when OpenapiService is deleted", func() {
			By("Deleting OpenapiService")
			ctx := context.Background()
			var openapiService appv1.OpenapiService
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      OpenapiServiceName,
				Namespace: OpenapiServiceNamespace,
			}, &openapiService)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, &openapiService)).Should(Succeed())

			By("Checking if WasmPlugin is deleted")
			Eventually(func() bool {
				var wasmPlugin unstructured.Unstructured
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "path-template-filter-" + OpenapiServiceName,
					Namespace: OpenapiServiceNamespace,
				}, &wasmPlugin)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})
})
