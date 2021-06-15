// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Component controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		ApplicationName    = "frontend-backend"
		ComponentName      = "test-component"
		ComponentNamespace = "default"
		JobName            = "test-job"
		KindName           = "radius.dev/Container@v1alpha1"
		Name               = "frontend"
		timeout            = time.Second * 10
		duration           = time.Second * 10
		interval           = time.Millisecond * 250
	)

	Context("When updating Component Status", func() {
		It("Should create Component", func() {
			By("By creating a new Component")
			ctx := context.Background()

			// apiVersion: applications.radius.dev/v1alpha1
			// kind: Application
			// metadata:
			//   name: radius-frontend-backend
			// spec: {}
			application := &v1alpha1.Application{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "applications.radius.dev/v1alpha1",
					Kind:       "Application",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("radius-%s", ApplicationName),
					Namespace: ComponentNamespace,
				},
			}

			img := map[string]interface{}{
				"image": "rynowak/frontend:0.5.0-dev",
			}

			Expect(k8sClient.Create(ctx, application)).Should(Succeed())

			run := map[string]interface{}{}
			run["container"] = img

			json, _ := json.Marshal(run)

			component := &v1alpha1.Component{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "applications.radius.dev/v1alpha1",
					Kind:       "Component",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      ComponentName,
					Namespace: ComponentNamespace,
					Annotations: map[string]string{
						"radius.dev/applications": ApplicationName,
						"radius.dev/components":   ComponentName,
					},
				},
				Spec: v1alpha1.ComponentSpec{
					Kind:     KindName,
					Run:      &runtime.RawExtension{Raw: json},
					Bindings: runtime.RawExtension{},
				},
			}
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			componentLookupKey := types.NamespacedName{Name: ComponentName, Namespace: ComponentNamespace}
			createdComponent := &v1alpha1.Component{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, componentLookupKey, createdComponent)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(createdComponent.Annotations["radius.dev/applications"]).Should(Equal(ApplicationName))
			Expect(createdComponent.Annotations["radius.dev/components"]).Should(Equal(ComponentName))
			Expect(createdComponent.Spec.Kind).Should(Equal(KindName))
			Expect(createdComponent.Spec.Run.MarshalJSON()).Should((Equal(json)))
		})
	})
})
