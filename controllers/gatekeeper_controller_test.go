/*
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

package controllers

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/openshift/library-go/pkg/manifest"
	admregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	operatorv1alpha1 "github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
	"github.com/gatekeeper/gatekeeper-operator/pkg/util"
	test "github.com/gatekeeper/gatekeeper-operator/test/util"
)

var namespace = "testns"

func TestDeployWebhookConfigs(t *testing.T) {
	g := NewWithT(t)
	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// Test default (nil) webhook configurations
	// ValidatingWebhookConfiguration nil
	// MutatingWebhookConfiguration nil
	g.Expect(getStaticAssets(gatekeeper)).To(ContainElement(ValidatingWebhookConfiguration))
	g.Expect(getStaticAssets(gatekeeper)).NotTo(ContainElement(MutatingWebhookConfiguration))

	webhookEnabled := operatorv1alpha1.WebhookEnabled
	webhookDisabled := operatorv1alpha1.WebhookDisabled

	// ValidatingWebhookConfiguration enabled
	// MutatingWebhookConfiguration enabled
	gatekeeper.Spec.ValidatingWebhook = &webhookEnabled
	gatekeeper.Spec.MutatingWebhook = &webhookEnabled
	g.Expect(getStaticAssets(gatekeeper)).To(ContainElement(ValidatingWebhookConfiguration))
	g.Expect(getStaticAssets(gatekeeper)).To(ContainElements(mutatingStaticAssets))

	// ValidatingWebhookConfiguration enabled
	// MutatingWebhookConfiguration disabled
	gatekeeper.Spec.ValidatingWebhook = &webhookEnabled
	gatekeeper.Spec.MutatingWebhook = &webhookDisabled
	g.Expect(getStaticAssets(gatekeeper)).To(ContainElement(ValidatingWebhookConfiguration))
	g.Expect(getStaticAssets(gatekeeper)).NotTo(ContainElements(mutatingStaticAssets))

	// ValidatingWebhookConfiguration disabled
	// MutatingWebhookConfiguration enabled
	gatekeeper.Spec.ValidatingWebhook = &webhookDisabled
	gatekeeper.Spec.MutatingWebhook = &webhookEnabled
	g.Expect(getStaticAssets(gatekeeper)).NotTo(ContainElement(ValidatingWebhookConfiguration))
	g.Expect(getStaticAssets(gatekeeper)).To(ContainElements(mutatingStaticAssets))

	// ValidatingWebhookConfiguration disabled
	// MutatingWebhookConfiguration disabled
	gatekeeper.Spec.ValidatingWebhook = &webhookDisabled
	gatekeeper.Spec.MutatingWebhook = &webhookDisabled
	g.Expect(getStaticAssets(gatekeeper)).NotTo(ContainElement(ValidatingWebhookConfiguration))
	g.Expect(getStaticAssets(gatekeeper)).NotTo(ContainElements(mutatingStaticAssets))
}

func TestGetSubsetOfAssets(t *testing.T) {
	g := NewWithT(t)
	g.Expect(getSubsetOfAssets(orderedStaticAssets)).To(Equal(orderedStaticAssets))
	g.Expect(getSubsetOfAssets(orderedStaticAssets, orderedStaticAssets...)).To(HaveLen(0))
	g.Expect(getSubsetOfAssets(orderedStaticAssets, mutatingStaticAssets...)).To(HaveLen(len(orderedStaticAssets) - len(mutatingStaticAssets)))
}

func TestCustomNamespace(t *testing.T) {
	g := NewWithT(t)
	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	expectedNamespace := "otherNamespace"
	// Rolebinding namespace overrides
	rolebindingManifest, err := util.GetManifest(RoleBindingFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(rolebindingManifest).ToNot(BeNil())
	err = crOverrides(gatekeeper, RoleBindingFile, rolebindingManifest, expectedNamespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	subjects, found, err := unstructured.NestedSlice(rolebindingManifest.Obj.Object, "subjects")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	for _, s := range subjects {
		subject := s.(map[string]interface{})
		g.Expect(subject).NotTo(BeNil())
		ns, found, err := unstructured.NestedString(subject, "namespace")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		g.Expect(ns).To(Equal(expectedNamespace))
	}

	// WebhookFile namespace overrides
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(webhookManifest).ToNot(BeNil())
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, expectedNamespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(getContainerArguments(g, managerContainer, webhookManifest)).To(HaveKeyWithValue(ExemptNamespaceArg, expectedNamespace))

	// ValidatingWebhookConfiguration and MutatingWebhookConfiguration namespace overrides
	webhookConfigs := []string{
		ValidatingWebhookConfiguration,
		MutatingWebhookConfiguration,
	}
	for _, webhookConfigFile := range webhookConfigs {
		webhookConfig, err := util.GetManifest(webhookConfigFile)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(webhookConfig).ToNot(BeNil())
		err = crOverrides(gatekeeper, webhookConfigFile, webhookConfig, expectedNamespace, false)
		g.Expect(err).ToNot(HaveOccurred())

		assertWebhooksWithFn(g, webhookConfig, func(webhook map[string]interface{}) {
			ns, found, err := unstructured.NestedString(webhook, "clientConfig", "service", "namespace")
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(found).To(BeTrue())
			g.Expect(ns).To(Equal(expectedNamespace))
		})
	}
}

func TestReplicas(t *testing.T) {
	g := NewWithT(t)
	auditReplicaOverride := int32(4)
	webhookReplicaOverride := int32(7)
	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default audit replicas
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(auditManifest).ToNot(BeNil())
	testManifestReplicas(g, auditManifest, test.DefaultDeployment.AuditReplicas)
	// test nil audit replicas
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	testManifestReplicas(g, auditManifest, test.DefaultDeployment.AuditReplicas)
	// test audit replicas override
	gatekeeper.Spec.Audit = &operatorv1alpha1.AuditConfig{Replicas: &auditReplicaOverride}
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	testManifestReplicas(g, auditManifest, auditReplicaOverride)

	// test default webhook replicas
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(webhookManifest).ToNot(BeNil())
	testManifestReplicas(g, webhookManifest, test.DefaultDeployment.WebhookReplicas)
	// test nil webhook replicas
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	testManifestReplicas(g, webhookManifest, test.DefaultDeployment.WebhookReplicas)
	// test webhook replicas override
	gatekeeper.Spec.Webhook = &operatorv1alpha1.WebhookConfig{Replicas: &webhookReplicaOverride}
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	testManifestReplicas(g, webhookManifest, webhookReplicaOverride)
}

func testManifestReplicas(g *WithT, manifest *manifest.Manifest, expectedReplicas int32) {
	replicas, found, err := unstructured.NestedInt64(manifest.Obj.Object, "spec", "replicas")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(int32(replicas)).To(BeIdenticalTo(expectedReplicas))
}

func TestAffinity(t *testing.T) {
	g := NewWithT(t)
	affinity := &corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"auditKey": "auditValue",
						},
					},
				},
			},
		},
	}
	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default affinity
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertAuditAffinity(g, auditManifest, nil)
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertWebhookAffinity(g, webhookManifest, nil)

	// test nil affinity
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertAuditAffinity(g, auditManifest, nil)
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertWebhookAffinity(g, webhookManifest, nil)

	// test affinity override
	gatekeeper.Spec.Affinity = affinity

	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertAuditAffinity(g, auditManifest, affinity)
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertWebhookAffinity(g, webhookManifest, affinity)
}

func assertAuditAffinity(g *WithT, manifest *manifest.Manifest, expected *corev1.Affinity) {
	g.Expect(manifest).ToNot(BeNil())
	current, found, err := unstructured.NestedFieldCopy(manifest.Obj.Object, "spec", "template", "spec", "affinity")
	g.Expect(err).ToNot(HaveOccurred())
	if expected == nil {
		g.Expect(found).To(BeFalse())
	} else {
		g.Expect(found).To(BeTrue())
		assertAffinity(g, expected, current)
	}
}

func assertWebhookAffinity(g *WithT, manifest *manifest.Manifest, expected *corev1.Affinity) {
	g.Expect(manifest).ToNot(BeNil())
	current, found, err := unstructured.NestedFieldCopy(manifest.Obj.Object, "spec", "template", "spec", "affinity")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	if expected == nil {
		assertAffinity(g, test.DefaultDeployment.Affinity, current)
	} else {
		assertAffinity(g, expected, current)
	}
}

func assertAffinity(g *WithT, expected *corev1.Affinity, current interface{}) {
	g.Expect(util.ToMap(expected)).To(BeEquivalentTo(util.ToMap(current)))
}

func TestNodeSelector(t *testing.T) {
	g := NewWithT(t)
	nodeSelector := map[string]string{
		"region": "emea",
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default nodeSelector
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertNodeSelector(g, auditManifest, nil)
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertNodeSelector(g, auditManifest, nil)
	assertNodeSelector(g, webhookManifest, nil)

	// test nil nodeSelector
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertNodeSelector(g, auditManifest, nil)
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertNodeSelector(g, webhookManifest, nil)

	// test nodeSelector override
	gatekeeper.Spec.NodeSelector = nodeSelector
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertNodeSelector(g, auditManifest, nodeSelector)
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertNodeSelector(g, webhookManifest, nodeSelector)
}

func assertNodeSelector(g *WithT, manifest *manifest.Manifest, expected map[string]string) {
	g.Expect(manifest).NotTo(BeNil())
	current, found, err := unstructured.NestedStringMap(manifest.Obj.Object, "spec", "template", "spec", "nodeSelector")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	if expected == nil {
		g.Expect(test.DefaultDeployment.NodeSelector).To(BeEquivalentTo(current))
	} else {
		g.Expect(expected).To(BeEquivalentTo(current))
	}
}

func TestPodAnnotations(t *testing.T) {
	g := NewWithT(t)
	podAnnotations := map[string]string{
		"my.annotation/foo":         "example",
		"some.other.annotation/bar": "baz",
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default podAnnotations
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertPodAnnotations(g, auditManifest, nil)
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertPodAnnotations(g, webhookManifest, nil)

	// test nil podAnnotations
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertPodAnnotations(g, auditManifest, nil)
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertPodAnnotations(g, webhookManifest, nil)

	// test podAnnotations override
	gatekeeper.Spec.PodAnnotations = podAnnotations
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertPodAnnotations(g, auditManifest, podAnnotations)
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertPodAnnotations(g, webhookManifest, podAnnotations)
}

func assertPodAnnotations(g *WithT, manifest *manifest.Manifest, expected map[string]string) {
	g.Expect(manifest).NotTo(BeNil())
	current, found, err := unstructured.NestedStringMap(manifest.Obj.Object, "spec", "template", "metadata", "annotations")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	if expected == nil {
		g.Expect(test.DefaultDeployment.PodAnnotations).To(BeEquivalentTo(current))
	} else {
		g.Expect(expected).To(BeEquivalentTo(current))
	}
}

func TestTolerations(t *testing.T) {
	g := NewWithT(t)
	tolerations := []corev1.Toleration{
		{
			Key:      "example",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "example2",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectPreferNoSchedule,
		},
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default tolerations
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertTolerations(g, auditManifest, nil)
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertTolerations(g, webhookManifest, nil)

	// test nil tolerations
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertTolerations(g, auditManifest, nil)
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertTolerations(g, webhookManifest, nil)

	// test tolerations override
	gatekeeper.Spec.Tolerations = tolerations
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertTolerations(g, auditManifest, tolerations)
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertTolerations(g, webhookManifest, tolerations)
}

func assertTolerations(g *WithT, manifest *manifest.Manifest, expected []corev1.Toleration) {
	g.Expect(manifest).NotTo(BeNil())
	current, found, err := unstructured.NestedSlice(manifest.Obj.Object, "spec", "template", "spec", "tolerations")
	g.Expect(err).ToNot(HaveOccurred())
	if expected == nil {
		g.Expect(found).To(BeFalse())
	} else {
		for i, toleration := range expected {
			g.Expect(util.ToMap(toleration)).To(BeEquivalentTo(current[i]))
		}
	}
}

func TestResources(t *testing.T) {
	g := NewWithT(t)
	audit := &operatorv1alpha1.AuditConfig{
		Resources: &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000M"),
				corev1.ResourceMemory: resource.MustParse("1024Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("512"),
			},
		},
	}
	webhook := &operatorv1alpha1.WebhookConfig{
		Resources: &corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2001M"),
				corev1.ResourceMemory: resource.MustParse("1025Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("201m"),
				corev1.ResourceMemory: resource.MustParse("513"),
			},
		},
	}
	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default resources
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertResources(g, auditManifest, nil)
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertResources(g, webhookManifest, nil)

	// test nil resources
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertResources(g, auditManifest, nil)
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertResources(g, webhookManifest, nil)

	// test resources override
	gatekeeper.Spec.Audit = audit
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertResources(g, auditManifest, audit.Resources)

	gatekeeper.Spec.Webhook = webhook
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertResources(g, webhookManifest, webhook.Resources)
}

func assertResources(g *WithT, manifest *manifest.Manifest, expected *corev1.ResourceRequirements) {
	g.Expect(manifest).NotTo(BeNil())
	containers, found, err := unstructured.NestedSlice(manifest.Obj.Object, "spec", "template", "spec", "containers")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())

	for _, c := range containers {
		current, found, err := unstructured.NestedMap(util.ToMap(c), "resources")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		if expected == nil {
			assertResource(g, test.DefaultDeployment.Resources, current)
		} else {
			assertResource(g, expected, current)
		}
	}
}

func assertResource(g *WithT, expected *corev1.ResourceRequirements, current map[string]interface{}) {
	g.Expect(expected.Limits.Cpu().Cmp(resource.MustParse(current["limits"].(map[string]interface{})["cpu"].(string)))).To(BeZero())
	g.Expect(expected.Limits.Memory().Cmp(resource.MustParse(current["limits"].(map[string]interface{})["memory"].(string)))).To(BeZero())
	g.Expect(expected.Requests.Cpu().Cmp(resource.MustParse(current["requests"].(map[string]interface{})["cpu"].(string)))).To(BeZero())
	g.Expect(expected.Requests.Memory().Cmp(resource.MustParse(current["requests"].(map[string]interface{})["memory"].(string)))).To(BeZero())
}

func TestImage(t *testing.T) {
	g := NewWithT(t)
	image := "mycustom-image/the-gatekeeper:v1.0.0"
	imagePullPolicy := corev1.PullIfNotPresent
	imageConfig := &operatorv1alpha1.ImageConfig{
		Image:           &image,
		ImagePullPolicy: &imagePullPolicy,
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default image
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertImage(g, auditManifest, nil)
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	assertImage(g, webhookManifest, nil)

	// test nil image
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertImage(g, auditManifest, nil)
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertImage(g, webhookManifest, nil)

	// test image override
	gatekeeper.Spec.Image = imageConfig
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertImage(g, auditManifest, imageConfig)
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertImage(g, webhookManifest, imageConfig)
}

func assertImage(g *WithT, manifest *manifest.Manifest, expected *operatorv1alpha1.ImageConfig) {
	g.Expect(manifest).NotTo(BeNil())
	defaultImage, defaultImagePullPolicy := getDefaultImageConfig(g, manifest)
	containers, found, err := unstructured.NestedSlice(manifest.Obj.Object, "spec", "template", "spec", "containers")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())

	for _, c := range containers {
		currentImage, found, err := unstructured.NestedString(util.ToMap(c), "image")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		if expected == nil {
			g.Expect(defaultImage).To(BeEquivalentTo(currentImage))
		} else {
			g.Expect(*expected.Image).To(BeEquivalentTo(currentImage))
		}
		currentImagePullPolicy, found, err := unstructured.NestedString(util.ToMap(c), "imagePullPolicy")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		if expected == nil {
			g.Expect(defaultImagePullPolicy).To(BeEquivalentTo(currentImagePullPolicy))
		} else {
			g.Expect(*expected.ImagePullPolicy).To(BeEquivalentTo(currentImagePullPolicy))
		}
	}
}

func getDefaultImageConfig(g *WithT, manifest *manifest.Manifest) (image string, imagePullPolicy corev1.PullPolicy) {
	containers, found, err := unstructured.NestedSlice(manifest.Obj.Object, "spec", "template", "spec", "containers")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(containers).NotTo(BeNil())
	g.Expect(containers).To(HaveLen(1))
	image, found, err = unstructured.NestedString(containers[0].(map[string]interface{}), "image")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(image).NotTo(BeNil())
	policy, found, err := unstructured.NestedString(containers[0].(map[string]interface{}), "imagePullPolicy")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(policy).NotTo(BeNil())
	imagePullPolicy = corev1.PullPolicy(policy)
	return image, imagePullPolicy
}

func TestFailurePolicy(t *testing.T) {
	g := NewWithT(t)

	failurePolicy := admregv1.Fail
	webhook := operatorv1alpha1.WebhookConfig{
		FailurePolicy: &failurePolicy,
	}
	mutatingWebhook := operatorv1alpha1.WebhookEnabled
	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: operatorv1alpha1.GatekeeperSpec{
			MutatingWebhook: &mutatingWebhook,
		},
	}
	// test default failurePolicy
	valManifest, err := util.GetManifest(ValidatingWebhookConfiguration)
	g.Expect(err).ToNot(HaveOccurred())
	assertFailurePolicy(g, valManifest, ValidationGatekeeperWebhook, nil)
	mutManifest, err := util.GetManifest(MutatingWebhookConfiguration)
	g.Expect(err).ToNot(HaveOccurred())
	assertFailurePolicy(g, mutManifest, MutatingWebhookConfiguration, nil)

	// test nil failurePolicy
	err = crOverrides(gatekeeper, ValidatingWebhookConfiguration, valManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertFailurePolicy(g, valManifest, ValidationGatekeeperWebhook, nil)
	err = crOverrides(gatekeeper, MutatingWebhookConfiguration, mutManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertFailurePolicy(g, mutManifest, MutationGatekeeperWebhook, nil)

	// test failurePolicy override
	gatekeeper.Spec.Webhook = &webhook
	err = crOverrides(gatekeeper, ValidatingWebhookConfiguration, valManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertFailurePolicy(g, valManifest, ValidationGatekeeperWebhook, &failurePolicy)
	err = crOverrides(gatekeeper, MutatingWebhookConfiguration, mutManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertFailurePolicy(g, mutManifest, MutationGatekeeperWebhook, &failurePolicy)
}

func assertFailurePolicy(g *WithT, manifest *manifest.Manifest, webhookName string, expected *admregv1.FailurePolicyType) {
	assertWebhooksWithFn(g, manifest, func(webhook map[string]interface{}) {
		if webhook["name"] == webhookName {
			current, found, err := unstructured.NestedString(webhook, "failurePolicy")
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(found).To(BeTrue())
			if expected == nil {
				g.Expect(test.DefaultDeployment.FailurePolicy).To(BeEquivalentTo(current))
			} else {
				g.Expect(*expected).To(BeEquivalentTo(current))
			}
		}
	})
}

func TestNamespaceSelector(t *testing.T) {
	g := NewWithT(t)

	namespaceSelector := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "admission.gatekeeper.sh/enabled",
				Operator: metav1.LabelSelectorOpExists,
			},
		},
	}
	webhook := operatorv1alpha1.WebhookConfig{
		NamespaceSelector: &namespaceSelector,
	}
	mutatingWebhook := operatorv1alpha1.WebhookEnabled
	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: operatorv1alpha1.GatekeeperSpec{
			MutatingWebhook: &mutatingWebhook,
		},
	}
	// test default namespaceSelector
	valManifest, err := util.GetManifest(ValidatingWebhookConfiguration)
	g.Expect(err).ToNot(HaveOccurred())
	assertNamespaceSelector(g, valManifest, ValidationGatekeeperWebhook, nil)
	mutManifest, err := util.GetManifest(MutatingWebhookConfiguration)
	g.Expect(err).ToNot(HaveOccurred())
	assertNamespaceSelector(g, mutManifest, MutationGatekeeperWebhook, nil)

	// test nil namespaceSelector
	err = crOverrides(gatekeeper, ValidatingWebhookConfiguration, valManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertNamespaceSelector(g, valManifest, ValidationGatekeeperWebhook, nil)
	err = crOverrides(gatekeeper, MutatingWebhookConfiguration, mutManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertNamespaceSelector(g, mutManifest, MutationGatekeeperWebhook, nil)

	// test namespaceSelector override
	gatekeeper.Spec.Webhook = &webhook
	err = crOverrides(gatekeeper, ValidatingWebhookConfiguration, valManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertNamespaceSelector(g, valManifest, ValidatingWebhookConfiguration, &namespaceSelector)
	err = crOverrides(gatekeeper, MutatingWebhookConfiguration, mutManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	assertNamespaceSelector(g, mutManifest, MutatingWebhookConfiguration, &namespaceSelector)
}

func assertNamespaceSelector(g *WithT, manifest *manifest.Manifest, webhookName string, expected *metav1.LabelSelector) {
	assertWebhooksWithFn(g, manifest, func(webhook map[string]interface{}) {
		if webhook["name"] == webhookName {
			current, found, err := unstructured.NestedFieldCopy(webhook, "namespaceSelector")
			g.Expect(err).ToNot(HaveOccurred())
			if expected == nil {
				// ValidatingWebhookConfiguration and
				// MutatingWebhookConfiguration have different defaults so the
				// following split is necessary.
				if webhookName == ValidationGatekeeperWebhook {
					g.Expect(found).To(BeTrue())
					g.Expect(util.ToMap(test.DefaultDeployment.NamespaceSelector)).To(BeEquivalentTo(current))
				} else {
					g.Expect(found).To(BeFalse())
					// Default NamespaceSelector for MutatingWebhookConfiguration
					// is nil and we already know expected is nil if we reach
					// this.
				}
			} else {
				g.Expect(util.ToMap(*expected)).To(BeEquivalentTo(current))
			}
		}
	})
}

func assertWebhooksWithFn(g *WithT, manifest *manifest.Manifest, webhookFn func(map[string]interface{})) {
	g.Expect(manifest).NotTo(BeNil())
	webhooks, found, err := unstructured.NestedSlice(manifest.Obj.Object, "webhooks")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(webhooks).ToNot(BeNil())

	for _, w := range webhooks {
		webhook := w.(map[string]interface{})
		webhookFn(webhook)
	}
}

func TestAuditInterval(t *testing.T) {
	g := NewWithT(t)
	auditInterval := metav1.Duration{
		Duration: time.Hour,
	}
	auditOverride := operatorv1alpha1.AuditConfig{
		AuditInterval: &auditInterval,
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(auditManifest).ToNot(BeNil())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditIntervalArg))
	// test nil
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditIntervalArg))
	// test override
	gatekeeper.Spec.Audit = &auditOverride
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(AuditIntervalArg, "3600"))
}

func TestAuditLogLevel(t *testing.T) {
	g := NewWithT(t)
	logLevel := operatorv1alpha1.LogLevelDEBUG
	auditOverride := operatorv1alpha1.AuditConfig{
		LogLevel: &logLevel,
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(auditManifest).ToNot(BeNil())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(LogLevelArg))
	// test nil
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(LogLevelArg))
	// test override
	gatekeeper.Spec.Audit = &auditOverride
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(LogLevelArg, "DEBUG"))
}

func TestAuditConstraintViolationLimit(t *testing.T) {
	g := NewWithT(t)
	constraintViolationLimit := uint64(20)
	auditOverride := operatorv1alpha1.AuditConfig{
		ConstraintViolationLimit: &constraintViolationLimit,
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(auditManifest).ToNot(BeNil())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(ConstraintViolationLimitArg))
	// test nil
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(ConstraintViolationLimitArg))
	// test override
	gatekeeper.Spec.Audit = &auditOverride
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(ConstraintViolationLimitArg, "20"))
}

func TestAuditChunkSize(t *testing.T) {
	g := NewWithT(t)
	auditChunkSize := uint64(10)
	auditOverride := operatorv1alpha1.AuditConfig{
		AuditChunkSize: &auditChunkSize,
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(auditManifest).ToNot(BeNil())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditChunkSizeArg))
	// test nil
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditChunkSizeArg))
	// test override
	gatekeeper.Spec.Audit = &auditOverride
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(AuditChunkSizeArg, "10"))
}

func TestAuditFromCache(t *testing.T) {
	g := NewWithT(t)
	auditFromCache := operatorv1alpha1.AuditFromCacheEnabled
	auditOverride := operatorv1alpha1.AuditConfig{
		AuditFromCache: &auditFromCache,
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(auditManifest).ToNot(BeNil())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditFromCacheArg))
	// test nil
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditFromCacheArg))
	// test override
	gatekeeper.Spec.Audit = &auditOverride
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(AuditFromCacheArg, "true"))
}

func TestEmitAuditEvents(t *testing.T) {
	g := NewWithT(t)
	emitEvents := operatorv1alpha1.EmitEventsEnabled
	auditOverride := operatorv1alpha1.AuditConfig{
		EmitAuditEvents: &emitEvents,
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(auditManifest).ToNot(BeNil())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(EmitAuditEventsArg))
	// test nil
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(EmitAuditEventsArg))
	// test override
	gatekeeper.Spec.Audit = &auditOverride
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(EmitAuditEventsArg, "true"))
}

func TestAllAuditArgs(t *testing.T) {
	g := NewWithT(t)
	auditChunkSize := uint64(10)
	auditFromCache := operatorv1alpha1.AuditFromCacheEnabled
	constraintViolationLimit := uint64(20)
	emitEvents := operatorv1alpha1.EmitEventsEnabled
	logLevel := operatorv1alpha1.LogLevelDEBUG
	auditInterval := metav1.Duration{
		Duration: time.Hour,
	}
	auditOverride := operatorv1alpha1.AuditConfig{
		AuditChunkSize:           &auditChunkSize,
		AuditFromCache:           &auditFromCache,
		ConstraintViolationLimit: &constraintViolationLimit,
		EmitAuditEvents:          &emitEvents,
		LogLevel:                 &logLevel,
		AuditInterval:            &auditInterval,
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default
	auditManifest, err := util.GetManifest(AuditFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(auditManifest).ToNot(BeNil())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditChunkSizeArg))
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditFromCacheArg))
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(ConstraintViolationLimitArg))
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(LogLevelArg))
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(EmitAuditEventsArg))
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditIntervalArg))
	// test nil
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditChunkSizeArg))
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditFromCacheArg))
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(ConstraintViolationLimitArg))
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(LogLevelArg))
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(EmitAuditEventsArg))
	expectManifestContainerArgument(g, managerContainer, auditManifest).NotTo(HaveKey(AuditIntervalArg))
	// test override
	gatekeeper.Spec.Audit = &auditOverride
	err = crOverrides(gatekeeper, AuditFile, auditManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(AuditChunkSizeArg, "10"))
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(AuditFromCacheArg, "true"))
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(ConstraintViolationLimitArg, "20"))
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(LogLevelArg, "DEBUG"))
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(EmitAuditEventsArg, "true"))
	expectManifestContainerArgument(g, managerContainer, auditManifest).To(HaveKeyWithValue(AuditIntervalArg, "3600"))
}

func TestEmitAdmissionEvents(t *testing.T) {
	g := NewWithT(t)
	emitEvents := operatorv1alpha1.EmitEventsEnabled
	webhookOverride := operatorv1alpha1.WebhookConfig{
		EmitAdmissionEvents: &emitEvents,
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(webhookManifest).ToNot(BeNil())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(EmitAdmissionEventsArg))
	// test nil
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(EmitAdmissionEventsArg))
	// test override
	gatekeeper.Spec.Webhook = &webhookOverride
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).To(HaveKeyWithValue(EmitAdmissionEventsArg, "true"))
}

func TestWebhookLogLevel(t *testing.T) {
	g := NewWithT(t)
	logLevel := operatorv1alpha1.LogLevelDEBUG
	webhookOverride := operatorv1alpha1.WebhookConfig{
		LogLevel: &logLevel,
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(webhookManifest).ToNot(BeNil())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(LogLevelArg))
	// test nil
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(LogLevelArg))
	// test override
	gatekeeper.Spec.Webhook = &webhookOverride
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).To(HaveKeyWithValue(LogLevelArg, "DEBUG"))
}

func TestMutationArg(t *testing.T) {
	g := NewWithT(t)

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}

	// test default
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(webhookManifest).ToNot(BeNil())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(EnableMutationArg))
	// test nil
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(EnableMutationArg))
	// test disabled override
	mutation := operatorv1alpha1.WebhookDisabled
	gatekeeper.Spec.MutatingWebhook = &mutation
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(EnableMutationArg))
	// test enabled override
	mutation = operatorv1alpha1.WebhookEnabled
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).To(HaveKeyWithValue(EnableMutationArg, "true"))
}

func TestAllWebhookArgs(t *testing.T) {
	g := NewWithT(t)
	emitEvents := operatorv1alpha1.EmitEventsEnabled
	logLevel := operatorv1alpha1.LogLevelDEBUG
	webhookOverride := operatorv1alpha1.WebhookConfig{
		EmitAdmissionEvents: &emitEvents,
		LogLevel:            &logLevel,
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default
	webhookManifest, err := util.GetManifest(WebhookFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(webhookManifest).ToNot(BeNil())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(EmitAdmissionEventsArg))
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(LogLevelArg))
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(EnableMutationArg))
	// test nil
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(EmitAdmissionEventsArg))
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(LogLevelArg))
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(EnableMutationArg))
	// test override without mutation
	gatekeeper.Spec.Webhook = &webhookOverride
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).To(HaveKeyWithValue(EmitAdmissionEventsArg, "true"))
	expectManifestContainerArgument(g, managerContainer, webhookManifest).To(HaveKeyWithValue(LogLevelArg, "DEBUG"))
	expectManifestContainerArgument(g, managerContainer, webhookManifest).NotTo(HaveKey(EnableMutationArg))
	// test override with mutation
	mutatingWebhook := operatorv1alpha1.WebhookEnabled
	gatekeeper.Spec.MutatingWebhook = &mutatingWebhook
	err = crOverrides(gatekeeper, WebhookFile, webhookManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	expectManifestContainerArgument(g, managerContainer, webhookManifest).To(HaveKeyWithValue(EmitAdmissionEventsArg, "true"))
	expectManifestContainerArgument(g, managerContainer, webhookManifest).To(HaveKeyWithValue(LogLevelArg, "DEBUG"))
	expectManifestContainerArgument(g, managerContainer, webhookManifest).To(HaveKeyWithValue(EnableMutationArg, "true"))
}

func expectManifestContainerArgument(g *WithT, containerName string, manifest *manifest.Manifest) Assertion {
	args := getContainerArguments(g, containerName, manifest)
	return g.Expect(args)
}

func getContainerArguments(g *WithT, containerName string, manifest *manifest.Manifest) map[string]string {
	containers, found, err := unstructured.NestedSlice(manifest.Obj.Object, "spec", "template", "spec", "containers")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())

	for _, c := range containers {
		cName, found, err := unstructured.NestedString(util.ToMap(c), "name")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		if cName == containerName {
			args, found, err := unstructured.NestedStringSlice(util.ToMap(c), "args")
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(found).To(BeTrue())
			argsMap := make(map[string]string)
			for _, arg := range args {
				key, value := util.FromArg(arg)
				argsMap[key] = value
			}
			return argsMap
		}
	}
	return nil
}

func TestSetCertNamespace(t *testing.T) {
	g := NewWithT(t)
	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	// test default
	serverCertManifest, err := util.GetManifest(ServerCertFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(serverCertManifest).ToNot(BeNil())
	err = crOverrides(gatekeeper, ServerCertFile, serverCertManifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(serverCertManifest.Obj.GetNamespace()).To(Equal(namespace))
}

func TestMutationRBACConfig(t *testing.T) {
	g := NewWithT(t)

	gatekeeper := &operatorv1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}

	clusterRoleManifest, err := util.GetManifest(ClusterRoleFile)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterRoleManifest).ToNot(BeNil())
	manifest := &manifest.Manifest{}

	// Test default RBAC config
	manifest.Obj = clusterRoleManifest.Obj.DeepCopy()
	err = crOverrides(gatekeeper, ClusterRoleFile, manifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	rules, found, err := unstructured.NestedSlice(manifest.Obj.Object, "rules")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(rules).NotTo(BeEmpty())
	for _, rule := range rules {
		r := rule.(map[string]interface{})
		for _, f := range matchMutatingRBACRuleFns {
			found, err := f(r)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(found).To(BeFalse())
		}
	}

	// Test RBAC config when mutating webhook mode is nil
	manifest.Obj = clusterRoleManifest.Obj.DeepCopy()
	err = crOverrides(gatekeeper, ClusterRoleFile, manifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	rules, found, err = unstructured.NestedSlice(manifest.Obj.Object, "rules")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(rules).NotTo(BeEmpty())
	for _, rule := range rules {
		r := rule.(map[string]interface{})
		for _, f := range matchMutatingRBACRuleFns {
			found, err := f(r)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(found).To(BeFalse())
		}
	}

	// Test RBAC config when mutating webhook mode is disabled
	manifest.Obj = clusterRoleManifest.Obj.DeepCopy()
	mutation := operatorv1alpha1.WebhookDisabled
	gatekeeper.Spec.MutatingWebhook = &mutation
	err = crOverrides(gatekeeper, ClusterRoleFile, manifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	rules, found, err = unstructured.NestedSlice(manifest.Obj.Object, "rules")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(rules).NotTo(BeEmpty())
	for _, rule := range rules {
		r := rule.(map[string]interface{})
		for _, f := range matchMutatingRBACRuleFns {
			found, err := f(r)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(found).To(BeFalse())
		}
	}

	// Test RBAC config when mutating webhook mode is enabled
	manifest.Obj = clusterRoleManifest.Obj.DeepCopy()
	mutation = operatorv1alpha1.WebhookEnabled
	gatekeeper.Spec.MutatingWebhook = &mutation
	err = crOverrides(gatekeeper, ClusterRoleFile, manifest, namespace, false)
	g.Expect(err).ToNot(HaveOccurred())
	rules, found, err = unstructured.NestedSlice(manifest.Obj.Object, "rules")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(rules).NotTo(BeEmpty())

	matchCount := 0
	for _, rule := range rules {
		r := rule.(map[string]interface{})
		for _, f := range matchMutatingRBACRuleFns {
			found, err := f(r)
			g.Expect(err).ToNot(HaveOccurred())
			if found {
				matchCount++
			}
		}
	}

	g.Expect(matchCount).To(Equal(len(matchMutatingRBACRuleFns)))
}
