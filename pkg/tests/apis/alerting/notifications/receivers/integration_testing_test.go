package receivers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/alerting/receivers/webhook"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/grafana/grafana/apps/alerting/notifications/pkg/apis/alertingnotifications/v0alpha1"
	"github.com/grafana/grafana/pkg/services/accesscontrol"
	"github.com/grafana/grafana/pkg/services/accesscontrol/resourcepermissions"
	"github.com/grafana/grafana/pkg/services/org"
	"github.com/grafana/grafana/pkg/tests/apis"
	"github.com/grafana/grafana/pkg/tsdb/cloudwatch/utils"
	"github.com/grafana/grafana/pkg/util/testutil"
)

const noUIDReceiver = "-"

func TestIntegrationTestingAccessControl(t *testing.T) {
	testutil.SkipIntegrationTestInShortMode(t)

	ctx := context.Background()
	helper := getTestHelper(t)

	org1 := helper.Org1

	adminClient, err := v0alpha1.NewReceiverClientFromGenerator(org1.Admin.GetClientRegistry())
	require.NoError(t, err)

	existing, err := adminClient.Create(ctx, &v0alpha1.Receiver{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "default",
		},
		Spec: v0alpha1.ReceiverSpec{
			Title: "receivers",
			Integrations: []v0alpha1.ReceiverIntegration{
				createIntegration(t, webhook.Type),
				createIntegration(t, webhook.Type),
				createIntegration(t, webhook.Type),
			},
		},
	}, resource.CreateOptions{})
	require.NoError(t, err)

	testAlert := v0alpha1.CreateReceiverIntegrationTestRequestAlert{
		Labels: map[string]string{
			"alertname": "test-alert",
		},
		Annotations: map[string]string{},
	}

	testCases := []struct {
		desc            string
		basicRole       org.RoleType
		permissions     []resourcepermissions.SetResourcePermissionCommand
		canTestNew      bool
		canTestExisting bool
	}{
		{
			desc:      "unauthorized cannot test",
			basicRole: org.RoleNone,
		},
		{
			desc:      "basic Viewer cannot test",
			basicRole: org.RoleViewer,
		},
		{
			desc:            "basic Editor can test",
			basicRole:       org.RoleEditor,
			canTestExisting: true,
			canTestNew:      true,
		},
		{
			desc:            "basic Admin can test",
			basicRole:       org.RoleAdmin,
			canTestExisting: true,
			canTestNew:      true,
		},
		{
			desc:      "legacy writer can test",
			basicRole: org.RoleNone,
			permissions: []resourcepermissions.SetResourcePermissionCommand{
				createWildcardPermission(accesscontrol.ActionAlertingNotificationsWrite),
			},
		},
		{
			desc:      "create + test can test new only",
			basicRole: org.RoleNone,
			permissions: []resourcepermissions.SetResourcePermissionCommand{
				createWildcardPermission(accesscontrol.ActionAlertingReceiversCreate, accesscontrol.ActionAlertingReceiversTest, accesscontrol.ActionAlertingReceiversRead),
			},
			canTestNew:      true,
			canTestExisting: false,
		},
		{
			desc:      "update + test can test existing only",
			basicRole: org.RoleNone,
			permissions: []resourcepermissions.SetResourcePermissionCommand{
				createWildcardPermission(accesscontrol.ActionAlertingReceiversTest, accesscontrol.ActionAlertingReceiversUpdate, accesscontrol.ActionAlertingReceiversRead),
			},
			canTestNew:      false,
			canTestExisting: true,
		},
	}

	for idx, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			var usr apis.User
			if tc.basicRole == org.RoleAdmin {
				usr = org1.Admin
			} else {
				usr = helper.CreateUser(fmt.Sprintf("user-%d", idx), apis.Org1, tc.basicRole, tc.permissions)
			}

			// Create v0alpha1 client for accessing the testing subresource
			receiverClient, err := v0alpha1.NewReceiverClientFromGenerator(usr.GetClientRegistry())
			require.NoError(t, err)

			t.Run("existing integration", func(t *testing.T) {
				request := v0alpha1.CreateReceiverIntegrationTestRequest{
					Body: v0alpha1.CreateReceiverIntegrationTestRequestBody{
						Integration: utils.Pointer(v0alpha1.CreateReceiverIntegrationTestRequestIntegration(existing.Spec.Integrations[1])),
						Alert:       testAlert,
					},
				}
				response, err := receiverClient.CreateReceiverIntegrationTest(ctx, existing.GetStaticMetadata().Identifier(), request)
				if tc.canTestExisting {
					require.NoError(t, err)
				} else {
					var data []byte
					if response != nil {
						data, err = json.Marshal(response)
					}
					require.Truef(t, errors.IsForbidden(err), "should get Forbidden error but got %s: %s", err, string(data))
				}
			})
			t.Run("new integration", func(t *testing.T) {
				newReceiver := resource.Identifier{Namespace: existing.Namespace, Name: noUIDReceiver}
				request := v0alpha1.CreateReceiverIntegrationTestRequest{
					Body: v0alpha1.CreateReceiverIntegrationTestRequestBody{
						Integration: utils.Pointer(v0alpha1.CreateReceiverIntegrationTestRequestIntegration(createIntegration(t, webhook.Type))),
						Alert:       testAlert,
					},
				}
				response, err := receiverClient.CreateReceiverIntegrationTest(ctx, newReceiver, request)
				if tc.canTestNew {
					require.NoError(t, err)
				} else {
					var data []byte
					if response != nil {
						data, err = json.Marshal(response)
					}
					require.Truef(t, errors.IsForbidden(err), "should get Forbidden error but got %s: %s", err, string(data))
				}
			})
		})
	}

	t.Run("creator can test its own", func(t *testing.T) {
		usr := helper.CreateUser("creator", apis.Org1, org.RoleViewer,
			[]resourcepermissions.SetResourcePermissionCommand{
				createWildcardPermission(accesscontrol.ActionAlertingReceiversTest, accesscontrol.ActionAlertingReceiversCreate),
			})
		receiverClient, err := v0alpha1.NewReceiverClientFromGenerator(usr.GetClientRegistry())
		require.NoError(t, err)
		rcv, err := receiverClient.Create(ctx, &v0alpha1.Receiver{
			ObjectMeta: v1.ObjectMeta{
				Namespace: "default",
			},
			Spec: v0alpha1.ReceiverSpec{
				Title: "creator's-receiver",
				Integrations: []v0alpha1.ReceiverIntegration{
					createIntegration(t, webhook.Type),
				},
			},
		}, resource.CreateOptions{})
		require.NoError(t, err)
		receiverID := resource.Identifier{Namespace: rcv.Namespace, Name: rcv.Name}
		request := v0alpha1.CreateReceiverIntegrationTestRequest{
			Body: v0alpha1.CreateReceiverIntegrationTestRequestBody{
				Integration: utils.Pointer(v0alpha1.CreateReceiverIntegrationTestRequestIntegration(rcv.Spec.Integrations[0])),
				Alert:       testAlert,
			},
		}
		_, err = receiverClient.CreateReceiverIntegrationTest(ctx, receiverID, request)
		require.NoError(t, err)
	})
	t.Run("when protected fields are changed", func(t *testing.T) {
		modified := existing.Spec.Integrations[0]
		modified.Settings = maps.Clone(modified.Settings)
		modified.Settings["url"] = "http://127.0.0.2"

		request := v0alpha1.CreateReceiverIntegrationTestRequest{
			Body: v0alpha1.CreateReceiverIntegrationTestRequestBody{
				Integration: utils.Pointer(v0alpha1.CreateReceiverIntegrationTestRequestIntegration(modified)),
				Alert:       testAlert,
			},
		}

		t.Run("should fail if no permissions", func(t *testing.T) {
			usr := helper.CreateUser("updater-no-protected", apis.Org1, org.RoleEditor,
				[]resourcepermissions.SetResourcePermissionCommand{
					createWildcardPermission(accesscontrol.ActionAlertingReceiversTest),
				})
			receiverClient, err := v0alpha1.NewReceiverClientFromGenerator(usr.GetClientRegistry())
			require.NoError(t, err)

			response, err := receiverClient.CreateReceiverIntegrationTest(ctx, existing.GetStaticMetadata().Identifier(), request)
			var data []byte
			if response != nil {
				data, err = json.Marshal(response)
			}
			require.Truef(t, errors.IsForbidden(err), "should get Forbidden error but got %s: %s", err, string(data))
		})
		t.Run("should pass if user has permissions", func(t *testing.T) {
			usr := helper.CreateUser("updater-with-protected", apis.Org1, org.RoleEditor,
				[]resourcepermissions.SetResourcePermissionCommand{
					createWildcardPermission(accesscontrol.ActionAlertingReceiversTest, accesscontrol.ActionAlertingReceiversUpdateProtected),
				})
			receiverClient, err := v0alpha1.NewReceiverClientFromGenerator(usr.GetClientRegistry())
			require.NoError(t, err)

			_, err = receiverClient.CreateReceiverIntegrationTest(ctx, existing.GetStaticMetadata().Identifier(), request)
			require.NoError(t, err)
		})
	})
}

func TestIntegrationTesting(t *testing.T) {
	t.Run("no receiver", func(t *testing.T) {
		t.Run("should test new integration", func(t *testing.T) {
		})
		t.Run("should fail if integration has UID", func(t *testing.T) {
		})
	})
	t.Run("existing receiver", func(t *testing.T) {
		t.Run("should accept existing integration", func(t *testing.T) {

		})
	})
}

type webhookReceiver struct {
	t      *testing.T
	server *http.Server

	receivedNotifications  map[string][]string
	responses              map[string]string
	notificationErrorCount int
	notificationsMtx       sync.RWMutex
}

func (w *webhookReceiver) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	w.t.Helper()
	w.notificationsMtx.Lock()
	defer w.notificationsMtx.Unlock()

	// paths[0] contains the receiver's name
	paths := strings.Split(request.URL.Path[1:], "/")

	key := strings.Join(paths[0:2], "/")
	b, err := io.ReadAll(request.Body)
	require.NoError(w.t, err)

	w.receivedNotifications[key] = append(w.receivedNotifications[key], string(b))
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(writer, w.responses[paths[0]])
}

func newWebhookReceiver(t *testing.T) *webhookReceiver {
	// Spin up a separate webserver to receive notifications emitted by Grafana.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	nc := &webhookReceiver{
		// Skip gosec linter since this is in test code.
		//
		//nolint:gosec
		server: &http.Server{
			Addr: listener.Addr().String(),
		},
		receivedNotifications: make(map[string][]string),
		responses:             make(map[string]string),
		t:                     t,
	}

	nc.server.Handler = nc
	go func() {
		require.EqualError(t, nc.server.Serve(listener), http.ErrServerClosed.Error())
	}()

	return nc
}
