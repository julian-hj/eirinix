package extension_test

import (
	"context"

	. "github.com/SUSE/eirinix"
	catalog "github.com/SUSE/eirinix/testing"
	. "github.com/onsi/ginkgo"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission/types"
)

var _ = Describe("Webhook implementation", func() {
	c := catalog.NewCatalog()
	m := c.SimpleManager()
	w := NewWebhook(c.SimpleExtension(), m)

	Context("With a fake extension", func() {
		It("It errors without a manager", func() {
			_, err := w.RegisterAdmissionWebHook(WebhookOptions{ID: "volume", ManagerOptions: ManagerOptions{Namespace: "eirini", OperatorFingerprint: "eirini-x"}})
			Expect(err.Error()).To(Equal("No failure policy set"))
			failurePolicy := admissionregistrationv1beta1.Fail

			_, err = w.RegisterAdmissionWebHook(WebhookOptions{ID: "volume", ManagerOptions: ManagerOptions{FailurePolicy: &failurePolicy, Namespace: "eirini", OperatorFingerprint: "eirini-x"}})
			Expect(err.Error()).To(Equal("couldn't build a new webhook: manager should be set using WithManager"))
		})

		It("Delegates to the Extension the handler", func() {
			ctx := context.Background()
			t := types.Request{}
			res := w.Handle(ctx, t)
			annotations := res.Response.AuditAnnotations
			v, ok := annotations["name"]
			Expect(ok).To(Equal(true))
			Expect(v).To(Equal("test"))
		})

	})
})
