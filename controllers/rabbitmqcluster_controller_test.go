/*
Copyright 2019 Pivotal.

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

package controllers_test

import (
	"context"
	"sync"
	"time"

	"github.com/pivotal/rabbitmq-for-kubernetes/controllers"

	"k8s.io/apimachinery/pkg/runtime"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	defaultscheme "k8s.io/client-go/kubernetes/scheme"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rabbitmqv1beta1 "github.com/pivotal/rabbitmq-for-kubernetes/api/v1beta1"

	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("RabbitmqclusterController", func() {
	Context("Reconcile", func() {
		var stopMgr chan struct{}
		var mgrStopped *sync.WaitGroup
		var client runtimeClient.Client
		var rabbitmqCluster *rabbitmqv1beta1.RabbitmqCluster
		var expectedRequest reconcile.Request
		var requests chan reconcile.Request
		var stsName types.NamespacedName
		var testReconciler reconcile.Reconciler
		const timeout = time.Millisecond * 700
		var scheme *runtime.Scheme
		var confMapName types.NamespacedName

		BeforeEach(func() {
			stsName = types.NamespacedName{Name: "p-foo", Namespace: "default"}
			confMapName = types.NamespacedName{Name: "rabbitmq-default-plugins", Namespace: "default"}

			expectedRequest = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "foo", Namespace: "default",
				},
			}

			rabbitmqCluster = &rabbitmqv1beta1.RabbitmqCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Spec: rabbitmqv1beta1.RabbitmqClusterSpec{
					Plan: "single",
				},
			}

			// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
			// channel when it is finished.

			scheme = runtime.NewScheme()
			Expect(rabbitmqv1beta1.AddToScheme(scheme)).NotTo(HaveOccurred())
			Expect(defaultscheme.AddToScheme(scheme)).NotTo(HaveOccurred())

			mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme})

			Expect(err).NotTo(HaveOccurred())
			client = mgr.GetClient()

			reconciler := &controllers.RabbitmqClusterReconciler{
				Client: client,
				Log:    ctrl.Log.WithName("controllers").WithName("rabbitmqcluster"),
				Scheme: mgr.GetScheme(),
			}

			testReconciler, requests = SetupTestReconcile(reconciler)

			err = ctrl.NewControllerManagedBy(mgr).
				For(&rabbitmqv1beta1.RabbitmqCluster{}).
				Complete(testReconciler)

			Expect(err).NotTo(HaveOccurred())

			stopMgr = make(chan struct{})
			mgrStopped = &sync.WaitGroup{}
			mgrStopped.Add(1)
			go func() {
				defer mgrStopped.Done()
				Expect(mgr.Start(stopMgr)).NotTo(HaveOccurred())
			}()

			Expect(client.Create(context.TODO(), rabbitmqCluster)).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(client.Delete(context.TODO(), rabbitmqCluster)).NotTo(HaveOccurred())
			close(stopMgr)
			mgrStopped.Wait()
		})

		It("Creates sts when rabbitmqcluster is created", func() {
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))

			clientSet, err := kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())

			sts, err := clientSet.AppsV1().StatefulSets("default").Get(stsName.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(sts.Name).To(Equal(stsName.Name))
		})

		It("Creates the configmap when rabbitmqcluster is created", func() {
			clientSet, err := kubernetes.NewForConfig(cfg)
			Expect(err).NotTo(HaveOccurred())

			configMap, err := clientSet.CoreV1().ConfigMaps("default").Get(confMapName.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(configMap.Name).To(Equal(confMapName.Name))
		})
	})
})

func SetupTestReconcile(inner reconcile.Reconciler) (reconcile.Reconciler, chan reconcile.Request) {
	requests := make(chan reconcile.Request)
	fn := reconcile.Func(func(req reconcile.Request) (reconcile.Result, error) {
		result, err := inner.Reconcile(req)
		requests <- req
		return result, err
	})
	return fn, requests
}