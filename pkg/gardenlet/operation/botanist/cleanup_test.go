// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package botanist_test

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"k8s.io/kube-aggregator/pkg/controllers/autoregister"
	testclock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	fakekubernetes "github.com/gardener/gardener/pkg/client/kubernetes/fake"
	"github.com/gardener/gardener/pkg/gardenlet/operation"
	. "github.com/gardener/gardener/pkg/gardenlet/operation/botanist"
	shootpkg "github.com/gardener/gardener/pkg/gardenlet/operation/shoot"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
)

var _ = Describe("Cleanup", func() {
	const namespace = "shoot--foo--bar"

	var (
		ctx = context.Background()

		fakeClock   *testclock.FakeClock
		shootClient client.Client
		seedClient  client.Client
		shoot       *gardencorev1beta1.Shoot
		botanist    *Botanist

		// managedByGardenerLabels are the labels a resource deployed via a ManagedResource carries in the shoot cluster.
		managedByGardenerLabels = map[string]string{resourcesv1alpha1.ManagedBy: resourcesv1alpha1.GardenerManager}
		// systemComponentLabels mark a resource as a Gardener system component.
		systemComponentLabels = map[string]string{v1beta1constants.GardenRole: v1beta1constants.GardenRoleSystemComponent}
		// noCleanupLabels opt a resource out of cleanup.
		noCleanupLabels = map[string]string{v1beta1constants.ShootNoCleanup: "true"}
	)

	// expectGone asserts that the given object no longer exists in the shoot cluster.
	expectGone := func(obj client.Object) {
		GinkgoHelper()
		Expect(shootClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(BeNotFoundError())
	}

	// expectPresent asserts that the given object still exists in the shoot cluster.
	expectPresent := func(obj client.Object) {
		GinkgoHelper()
		Expect(shootClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
	}

	// newService returns a Service in the 'default' namespace with the given name and labels.
	newService := func(name string, labels map[string]string) *corev1.Service {
		return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: labels}}
	}

	BeforeEach(func() {
		fakeClock = testclock.NewFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		shootClient = fakeclient.NewClientBuilder().WithScheme(kubernetes.ShootScheme).Build()
		shootClientSet := fakekubernetes.NewClientSetBuilder().WithClient(shootClient).Build()

		seedNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		seedClient = fakeclient.NewClientBuilder().WithScheme(kubernetes.SeedScheme).WithObjects(seedNamespace).Build()
		seedClientSet := fakekubernetes.NewClientSetBuilder().WithClient(seedClient).Build()

		shoot = &gardencorev1beta1.Shoot{ObjectMeta: metav1.ObjectMeta{Name: "bar", Namespace: "garden-foo"}}

		botanist = &Botanist{
			Operation: &operation.Operation{
				Logger:              logr.Discard(),
				Clock:               fakeClock,
				ShootClientSet:      shootClientSet,
				SeedClientSet:       seedClientSet,
				SeedNamespaceObject: seedNamespace,
				Shoot: &shootpkg.Shoot{
					ControlPlaneNamespace: namespace,
				},
			},
		}
		botanist.Shoot.SetInfo(shoot)

		// The shoot resources being cleaned up live in a namespace that exists in the shoot cluster, so they
		// are not ignored as leftovers in unknown namespaces during the cleanup.
		Expect(shootClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})).To(Succeed())
	})

	Describe("#CleanWebhooks", func() {
		It("should delete user webhooks but keep managed, system component and no-cleanup webhooks", func() {
			userMutatingWebhook := &admissionregistrationv1.MutatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "user"}}
			managedMutatingWebhook := &admissionregistrationv1.MutatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "managed", Labels: managedByGardenerLabels}}
			userValidatingWebhook := &admissionregistrationv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "user"}}
			managedValidatingWebhook := &admissionregistrationv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "managed", Labels: managedByGardenerLabels}}
			systemComponentWebhook := &admissionregistrationv1.MutatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "system", Labels: systemComponentLabels}}
			noCleanupWebhook := &admissionregistrationv1.MutatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "no-cleanup", Labels: noCleanupLabels}}

			for _, obj := range []client.Object{userMutatingWebhook, managedMutatingWebhook, userValidatingWebhook, managedValidatingWebhook, systemComponentWebhook, noCleanupWebhook} {
				Expect(shootClient.Create(ctx, obj)).To(Succeed())
			}

			Expect(botanist.CleanWebhooks(ctx)).To(Succeed())

			expectGone(userMutatingWebhook)
			expectGone(userValidatingWebhook)
			expectPresent(managedMutatingWebhook)
			expectPresent(managedValidatingWebhook)
			expectPresent(systemComponentWebhook)
			expectPresent(noCleanupWebhook)
		})

		It("should return an error if the finalize-grace-period annotation is not an integer", func() {
			shoot.Annotations = map[string]string{v1beta1constants.AnnotationShootCleanupWebhooksFinalizeGracePeriodSeconds: "not-a-number"}
			botanist.Shoot.SetInfo(shoot)

			Expect(botanist.CleanWebhooks(ctx)).To(HaveOccurred())
		})
	})

	Describe("#CleanExtendedAPIs", func() {
		It("should delete user CRDs and APIServices but keep managed and system component ones", func() {
			userCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "user.example.com"}}
			managedCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "managed.example.com", Labels: managedByGardenerLabels}}
			systemComponentCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "system.example.com", Labels: systemComponentLabels}}
			userAPIService := &apiregistrationv1.APIService{ObjectMeta: metav1.ObjectMeta{Name: "v1.user.example.com"}}
			managedAPIService := &apiregistrationv1.APIService{ObjectMeta: metav1.ObjectMeta{Name: "v1.managed.example.com", Labels: managedByGardenerLabels}}
			systemComponentAPIService := &apiregistrationv1.APIService{ObjectMeta: metav1.ObjectMeta{Name: "v1.system.example.com", Labels: systemComponentLabels}}

			for _, obj := range []client.Object{userCRD, managedCRD, systemComponentCRD, userAPIService, managedAPIService, systemComponentAPIService} {
				Expect(shootClient.Create(ctx, obj)).To(Succeed())
			}

			Expect(botanist.CleanExtendedAPIs(ctx)).To(Succeed())

			expectGone(userCRD)
			expectGone(userAPIService)
			expectPresent(managedCRD)
			expectPresent(managedAPIService)
			expectPresent(systemComponentCRD)
			expectPresent(systemComponentAPIService)
		})

		It("should keep APIServices that are auto-managed by kube-aggregator", func() {
			autoManagedAPIService := &apiregistrationv1.APIService{ObjectMeta: metav1.ObjectMeta{
				Name:   "v1.automanaged.example.com",
				Labels: map[string]string{autoregister.AutoRegisterManagedLabel: "true"},
			}}
			Expect(shootClient.Create(ctx, autoManagedAPIService)).To(Succeed())

			Expect(botanist.CleanExtendedAPIs(ctx)).To(Succeed())

			expectPresent(autoManagedAPIService)
		})

		It("should return an error if the finalize-grace-period annotation is not an integer", func() {
			shoot.Annotations = map[string]string{v1beta1constants.AnnotationShootCleanupExtendedAPIsFinalizeGracePeriodSeconds: "not-a-number"}
			botanist.Shoot.SetInfo(shoot)

			Expect(botanist.CleanExtendedAPIs(ctx)).To(HaveOccurred())
		})
	})

	Describe("#CleanKubernetesResources", func() {
		It("should delete all cleanable resource kinds across the shoot cluster", func() {
			cronJob := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: "cronjob", Namespace: "default"}}
			daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "daemonset", Namespace: "default"}}
			deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deployment", Namespace: "default"}}
			ingress := &networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: "default"}}
			job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", Namespace: "default"}}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "default"}}
			replicaSet := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "replicaset", Namespace: "default"}}
			replicationController := &corev1.ReplicationController{ObjectMeta: metav1.ObjectMeta{Name: "rc", Namespace: "default"}}
			service := newService("service", nil)
			statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "statefulset", Namespace: "default"}}
			persistentVolumeClaim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc", Namespace: "default"}}

			objects := []client.Object{cronJob, daemonSet, deployment, ingress, job, pod, replicaSet, replicationController, service, statefulSet, persistentVolumeClaim}
			for _, obj := range objects {
				Expect(shootClient.Create(ctx, obj)).To(Succeed())
			}

			Expect(botanist.CleanKubernetesResources(ctx)).To(Succeed())

			for _, obj := range objects {
				expectGone(obj)
			}
		})

		It("should delete user resources but keep managed, system component and no-cleanup resources", func() {
			userDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "user", Namespace: "default"}}
			managedDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "managed", Namespace: "default", Labels: managedByGardenerLabels}}
			systemComponentDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "system", Namespace: "default", Labels: systemComponentLabels}}
			noCleanupDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "no-cleanup", Namespace: "default", Labels: noCleanupLabels}}

			userService := newService("user", nil)
			managedService := newService("managed", managedByGardenerLabels)
			otherManagedService := newService("other-managed", map[string]string{resourcesv1alpha1.ManagedBy: "some-other-manager"})
			kubernetesService := newService("kubernetes", map[string]string{Provider: KubernetesProvider})

			for _, obj := range []client.Object{userDeployment, managedDeployment, systemComponentDeployment, noCleanupDeployment, userService, managedService, otherManagedService, kubernetesService} {
				Expect(shootClient.Create(ctx, obj)).To(Succeed())
			}

			Expect(botanist.CleanKubernetesResources(ctx)).To(Succeed())

			expectGone(userDeployment)
			expectGone(userService)
			expectGone(otherManagedService)
			expectPresent(managedDeployment)
			expectPresent(managedService)
			expectPresent(systemComponentDeployment)
			expectPresent(noCleanupDeployment)
			expectPresent(kubernetesService)
		})

		Context("when the shoot has the skip-cleanup annotation", func() {
			BeforeEach(func() {
				shoot.Annotations = map[string]string{v1beta1constants.AnnotationShootSkipCleanup: "true"}
				botanist.Shoot.SetInfo(shoot)
			})

			It("should only clean Services and PersistentVolumeClaims, leaving workloads untouched", func() {
				service := newService("service", nil)
				persistentVolumeClaim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc", Namespace: "default"}}
				deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deployment", Namespace: "default"}}
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "default"}}
				for _, obj := range []client.Object{service, persistentVolumeClaim, deployment, pod} {
					Expect(shootClient.Create(ctx, obj)).To(Succeed())
				}

				Expect(botanist.CleanKubernetesResources(ctx)).To(Succeed())

				expectGone(service)
				expectGone(persistentVolumeClaim)
				expectPresent(deployment)
				expectPresent(pod)
			})
		})

		Context("cleanup start time annotation", func() {
			It("should record the cleanup start time on the shoot namespace when it is not yet set", func() {
				Expect(botanist.CleanKubernetesResources(ctx)).To(Succeed())

				seedNamespace := &corev1.Namespace{}
				Expect(seedClient.Get(ctx, client.ObjectKey{Name: namespace}, seedNamespace)).To(Succeed())
				Expect(seedNamespace.Annotations).To(HaveKeyWithValue(v1beta1constants.AnnotationShootCleanupKubernetesResourcesStartTime, fakeClock.Now().UTC().Format(time.RFC3339)))
			})

			It("should preserve an already recorded cleanup start time", func() {
				existingStartTime := fakeClock.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
				seedNamespace := &corev1.Namespace{}
				Expect(seedClient.Get(ctx, client.ObjectKey{Name: namespace}, seedNamespace)).To(Succeed())
				metav1.SetMetaDataAnnotation(&seedNamespace.ObjectMeta, v1beta1constants.AnnotationShootCleanupKubernetesResourcesStartTime, existingStartTime)
				Expect(seedClient.Update(ctx, seedNamespace)).To(Succeed())

				Expect(botanist.CleanKubernetesResources(ctx)).To(Succeed())

				Expect(seedClient.Get(ctx, client.ObjectKey{Name: namespace}, seedNamespace)).To(Succeed())
				Expect(seedNamespace.Annotations).To(HaveKeyWithValue(
					v1beta1constants.AnnotationShootCleanupKubernetesResourcesStartTime,
					existingStartTime,
				))
			})

			It("should return an error if the recorded cleanup start time cannot be parsed", func() {
				seedNamespace := &corev1.Namespace{}
				Expect(seedClient.Get(ctx, client.ObjectKey{Name: namespace}, seedNamespace)).To(Succeed())
				metav1.SetMetaDataAnnotation(&seedNamespace.ObjectMeta, v1beta1constants.AnnotationShootCleanupKubernetesResourcesStartTime, "not-a-timestamp")
				Expect(seedClient.Update(ctx, seedNamespace)).To(Succeed())

				Expect(botanist.CleanKubernetesResources(ctx)).To(HaveOccurred())
			})

			It("should ignore objects created after the cleanup start time instead of waiting for them to be gone", func() {
				deploymentCreatedAfterWindow := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "created-after-window",
						Namespace:         "default",
						Finalizers:        []string{"foo.bar.io/foobar"},
						CreationTimestamp: metav1.NewTime(fakeClock.Now().Add(time.Hour)),
					},
				}
				Expect(shootClient.Create(ctx, deploymentCreatedAfterWindow)).To(Succeed())

				Expect(botanist.CleanKubernetesResources(ctx)).To(Succeed())

				deploymentCreatedAfterWindow = &appsv1.Deployment{}
				Expect(shootClient.Get(ctx, client.ObjectKey{Name: "created-after-window", Namespace: "default"}, deploymentCreatedAfterWindow)).To(Succeed())
				Expect(deploymentCreatedAfterWindow.DeletionTimestamp).NotTo(BeNil())
			})
		})

		It("should return an error if the finalize-grace-period annotation is not an integer", func() {
			shoot.Annotations = map[string]string{v1beta1constants.AnnotationShootCleanupKubernetesResourcesFinalizeGracePeriodSeconds: "not-a-number"}
			botanist.Shoot.SetInfo(shoot)

			Expect(botanist.CleanKubernetesResources(ctx)).To(HaveOccurred())
		})
	})

	Describe("#CleanVolumeAttachments", func() {
		It("should delete all VolumeAttachments regardless of their labels", func() {
			plainVolumeAttachment := &storagev1.VolumeAttachment{
				ObjectMeta: metav1.ObjectMeta{Name: "plain"},
				Spec:       storagev1.VolumeAttachmentSpec{Attacher: "csi", NodeName: "node"},
			}
			managedVolumeAttachment := &storagev1.VolumeAttachment{
				ObjectMeta: metav1.ObjectMeta{Name: "managed", Labels: managedByGardenerLabels},
				Spec:       storagev1.VolumeAttachmentSpec{Attacher: "csi", NodeName: "node"},
			}
			Expect(shootClient.Create(ctx, plainVolumeAttachment)).To(Succeed())
			Expect(shootClient.Create(ctx, managedVolumeAttachment)).To(Succeed())

			Expect(CleanVolumeAttachments(ctx, logr.Discard(), shootClient)).To(Succeed())

			expectGone(plainVolumeAttachment)
			expectGone(managedVolumeAttachment)
		})
	})
})
