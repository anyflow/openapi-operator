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
	"fmt"
	"path"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appv1 "github.com/anyflow/openapi-operator/api/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// OpenapiServiceReconciler reconciles a OpenapiService object
type OpenapiServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.anyflow.net,resources=openapiservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.anyflow.net,resources=openapiservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.anyflow.net,resources=openapiservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=extensions.istio.io,resources=wasmplugins,verbs=get;list;watch;create;update;patch;delete

// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile
func (r *OpenapiServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling OpenapiService", "request", req.NamespacedName)

	// OpenapiService 가져오기
	var openapiService appv1.OpenapiService
	if err := r.Get(ctx, req.NamespacedName, &openapiService); err != nil {
		if errors.IsNotFound(err) {
			// 오브젝트가 삭제됨 - 추가 처리 필요 없음
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get OpenapiService")
		return ctrl.Result{}, err
	}

	// 이전 WasmPlugin 정리
	if err := r.cleanupOldWasmPlugin(ctx, &openapiService); err != nil {
		logger.Error(err, "Failed to cleanup old WasmPlugin")
		return ctrl.Result{}, err
	}

	// 새 WasmPlugin 생성
	wasmPlugin, err := r.createWasmPlugin(ctx, &openapiService)
	if err != nil {
		logger.Error(err, "Failed to create WasmPlugin")
		return ctrl.Result{}, err
	}

	// 상태 업데이트
	openapiService.Status.WasmPluginName = fmt.Sprintf("%s/%s", wasmPlugin.GetNamespace(), wasmPlugin.GetName())
	if err := r.Status().Update(ctx, &openapiService); err != nil {
		logger.Error(err, "Failed to update OpenapiService status")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciliation completed", "wasmPlugin", openapiService.Status.WasmPluginName)
	return ctrl.Result{}, nil
}

// createWasmPlugin OpenapiService에서 WasmPlugin 생성
func (r *OpenapiServiceReconciler) createWasmPlugin(ctx context.Context, openapiService *appv1.OpenapiService) (*unstructured.Unstructured, error) {
	logger := log.FromContext(ctx)

	// WasmPlugin 이름 생성
	pluginName := fmt.Sprintf("path-template-filter-%s", openapiService.Name)

	// 경로 처리
	paths := make(map[string]struct{})
	for pathKey := range openapiService.Spec.OpenAPI.Paths {
		if openapiService.Spec.Prefix != "" {
			pathKey = path.Join("/", openapiService.Spec.Prefix, pathKey)
		}
		paths[pathKey] = struct{}{}
	}

	// WasmPlugin 객체 생성
	wasmPlugin := &unstructured.Unstructured{}
	wasmPlugin.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "extensions.istio.io",
		Version: "v1alpha1",
		Kind:    "WasmPlugin",
	})
	wasmPlugin.SetName(pluginName)
	wasmPlugin.SetNamespace(openapiService.Namespace)

	// Spec 설정
	spec := map[string]interface{}{
		"url":              "anyflow/path-template-filter:0.2.2",
		"imagePullPolicy":  "Always",
		"phase":           "STATS",
		"failStrategy":    "FAIL_OPEN",
		"priority":        10,
		"selector":        openapiService.Spec.Selector,
		"pluginConfig": map[string]interface{}{
			"cacheSize": float64(5),
			"services": []map[string]interface{}{
				{
					"name":  openapiService.Spec.Prefix,
					"paths": paths,
				},
			},
		},
	}
	wasmPlugin.Object["spec"] = spec

	// 컨트롤러 참조 설정
	if err := controllerutil.SetControllerReference(openapiService, wasmPlugin, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	// WasmPlugin 생성 또는 업데이트
	existingWasmPlugin := &unstructured.Unstructured{}
	existingWasmPlugin.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "extensions.istio.io",
		Version: "v1alpha1",
		Kind:    "WasmPlugin",
	})
	err := r.Get(ctx, types.NamespacedName{Name: wasmPlugin.GetName(), Namespace: wasmPlugin.GetNamespace()}, existingWasmPlugin)

	if err != nil {
		if errors.IsNotFound(err) {
			// WasmPlugin이 존재하지 않으면 생성
			if err := r.Create(ctx, wasmPlugin); err != nil {
				return nil, fmt.Errorf("failed to create WasmPlugin: %w", err)
			}
			logger.Info("Created WasmPlugin", "name", wasmPlugin.GetName(), "namespace", wasmPlugin.GetNamespace())
		} else {
			return nil, fmt.Errorf("failed to get existing WasmPlugin: %w", err)
		}
	} else {
		// 기존 WasmPlugin 업데이트
		existingWasmPlugin.Object["spec"] = spec
		if err := r.Update(ctx, existingWasmPlugin); err != nil {
			return nil, fmt.Errorf("failed to update WasmPlugin: %w", err)
		}
		logger.Info("Updated WasmPlugin", "name", wasmPlugin.GetName(), "namespace", wasmPlugin.GetNamespace())
		wasmPlugin = existingWasmPlugin
	}

	return wasmPlugin, nil
}

// cleanupOldWasmPlugin 상태에 있는 이전 WasmPlugin 정리
func (r *OpenapiServiceReconciler) cleanupOldWasmPlugin(ctx context.Context, openapiService *appv1.OpenapiService) error {
	logger := log.FromContext(ctx)

	if openapiService.Status.WasmPluginName == "" {
		return nil
	}

	wasmPlugin := &unstructured.Unstructured{}
	wasmPlugin.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "extensions.istio.io",
		Version: "v1alpha1",
		Kind:    "WasmPlugin",
	})
	wasmPlugin.SetName(fmt.Sprintf("path-template-filter-%s", openapiService.Name))
	wasmPlugin.SetNamespace(openapiService.Namespace)

	if err := r.Delete(ctx, wasmPlugin); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "Failed to delete old WasmPlugin")
		return fmt.Errorf("failed to delete old WasmPlugin: %w", err)
	}

	logger.Info("Deleted old WasmPlugin", "name", wasmPlugin.GetName(), "namespace", wasmPlugin.GetNamespace())
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OpenapiServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1.OpenapiService{}).
		Named("openapiservice").
		Complete(r)
}
