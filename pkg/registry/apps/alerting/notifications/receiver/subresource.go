package receiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/grafana/grafana-app-sdk/app"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana/apps/alerting/notifications/pkg/apis/alertingnotifications/v0alpha1"
	"github.com/grafana/grafana/pkg/apimachinery/identity"
	"github.com/grafana/grafana/pkg/services/ngalert/models"
	"github.com/grafana/grafana/pkg/services/ngalert/notifier"
	"github.com/grafana/grafana/pkg/util/errhttp"
)

const newReceiverNamePlaceholder = "-"

type testingService interface {
	PatchIntegrationAndTest(ctx context.Context, user identity.Requester, alert notifier.Alert, receiverUID string, integration models.Integration, requiredSecrets []string) (notifier.IntegrationTestResult, error)
	TestByIntegrationUID(ctx context.Context, user identity.Requester, alert notifier.Alert, receiverUID string, integrationUID string) (notifier.IntegrationTestResult, error)
	TestNewReceiverIntegration(ctx context.Context, user identity.Requester, alert notifier.Alert, receiverUID string, integration models.Integration) (notifier.IntegrationTestResult, error)
}

type RequestHandler struct {
	testingSvc testingService
}

func New(svc testingService) *RequestHandler {
	return &RequestHandler{
		testingSvc: svc,
	}
}

func (p *RequestHandler) HandleReceiverTestingRequest(ctx context.Context, w app.CustomRouteResponseWriter, r *app.CustomRouteRequest) error {
	user, err := identity.GetRequester(ctx)
	if err != nil {
		return err
	}
	var req v0alpha1.CreateReceiverIntegrationTestRequestBody
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return err
	}

	result, err := p.TestReceiver(ctx, user, r.ResourceIdentifier.Name, req)
	if err != nil {
		errhttp.Write(ctx, err, w)
		return nil
	}

	responseBody := v0alpha1.CreateReceiverIntegrationTestBody{
		Duration: result.LastNotifyAttemptDuration,
		Status:   v0alpha1.CreateReceiverIntegrationTestBodyStatusSuccess,
	}
	if result.LastNotifyAttemptError != "" {
		responseBody.Status = v0alpha1.CreateReceiverIntegrationTestBodyStatusFailure
		responseBody.Error = &result.LastNotifyAttemptError
	}
	json, err := json.Marshal(v0alpha1.CreateReceiverIntegrationTest{
		TypeMeta:                          metav1.TypeMeta{},
		CreateReceiverIntegrationTestBody: responseBody,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	w.WriteHeader(200)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(json)
	return nil
}

func (p *RequestHandler) TestReceiver(ctx context.Context, user identity.Requester, receiverUID string, body v0alpha1.CreateReceiverIntegrationTestRequestBody) (notifier.IntegrationTestResult, error) {
	if receiverUID == newReceiverNamePlaceholder {
		receiverUID = ""
	}
	if err := validateCreateReceiverIntegrationTestRequestBody(receiverUID, body); err != nil {
		return notifier.IntegrationTestResult{}, notifier.ErrInvalidIntegrationTest.Errorf(err.Error())
	}
	alert := notifier.Alert(body.Alert)
	if body.IntegrationRef != nil {
		return p.testingSvc.TestByIntegrationUID(ctx, user, alert, receiverUID, body.IntegrationRef.Uid)
	}
	if body.Integration != nil {
		integration, secure, err := ConvertReceiverIntegrationToIntegration("", v0alpha1.ReceiverIntegration(*body.Integration))
		if err != nil {
			return notifier.IntegrationTestResult{}, err
		}
		if integration.UID == "" && len(secure) > 0 {
			return notifier.IntegrationTestResult{}, notifier.ErrInvalidIntegrationTest.Errorf("integration must have a UID to be tested with patched secure settings")
		}
		if integration.UID == "" {
			return p.testingSvc.TestNewReceiverIntegration(ctx, user, alert, receiverUID, integration)
		}
		return p.testingSvc.PatchIntegrationAndTest(ctx, user, alert, receiverUID, integration, secure)
	}
	return notifier.IntegrationTestResult{}, notifier.ErrInvalidIntegrationTest.Errorf("integrationRef or integration must be set")
}

func validateCreateReceiverIntegrationTestRequestBody(receiverUID string, body v0alpha1.CreateReceiverIntegrationTestRequestBody) error {
	if receiverUID == "" {
		if body.Integration.Uid != nil {
			return errors.New("integration UID must be empty when testing a new receiver")
		}
	}
	if body.Integration != nil && body.IntegrationRef != nil {
		return errors.New("integrationRef and integration cannot be set at the same time")
	}
	if body.Integration == nil && body.IntegrationRef == nil {
		return errors.New("integrationRef or integration must be set")
	}
	if body.IntegrationRef != nil && body.IntegrationRef.Uid == "" {
		return errors.New("integrationRef UID must be set")
	}
	if body.Integration != nil && (body.Integration.Uid == nil || *body.Integration.Uid == "") && len(body.Integration.SecureFields) > 0 {
		return errors.New("integration must have a UID to be tested with patched secure settings")
	}
	return nil
}
