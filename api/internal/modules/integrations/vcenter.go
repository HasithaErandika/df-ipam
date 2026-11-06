package integrations

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"dfipam/api/internal/platform/safehttp"

	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/soap"
)

// vCenter reads inventory through property retrieval only. It starts no tasks. The
// container view it creates lives in the session and changes nothing in vCenter.
type vCenter struct {
	url                *url.URL
	username, password string
	policy             safehttp.Policy
}

func newVCenter(c Connection, credentials map[string]string, policy safehttp.Policy) (Connector, error) {
	base, err := policy.CheckURL(c.BaseURL)
	if err != nil {
		return nil, err
	}
	return &vCenter{base.JoinPath("/sdk"), credentials["username"], credentials["password"], policy}, nil
}

func (v *vCenter) connect(ctx context.Context) (*vim25.Client, func(), error) {
	sc := soap.NewClient(v.url, false)
	tr := sc.DefaultTransport()
	tr.DialContext = v.policy.DialContext
	tr.Proxy = nil
	if v.policy.RootCAs != nil {
		tr.TLSClientConfig.RootCAs = v.policy.RootCAs
	}
	client, err := vim25.NewClient(ctx, sc)
	if err != nil {
		return nil, nil, cleanSOAP(err)
	}
	sessions := session.NewManager(client)
	if err := sessions.Login(ctx, url.UserPassword(v.username, v.password)); err != nil {
		return nil, nil, errors.New("vCenter refused the credentials")
	}
	return client, func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = sessions.Logout(ctx)
	}, nil
}

func cleanSOAP(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func (v *vCenter) Test(ctx context.Context) error {
	_, done, err := v.connect(ctx)
	if err != nil {
		return err
	}
	done()
	return nil
}

func (v *vCenter) Collect(ctx context.Context) (*Result, error) {
	client, done, err := v.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	return collectVCenter(ctx, client, time.Now())
}

func retrieve[T any](ctx context.Context, m *view.Manager, c *vim25.Client, kind string, props []string) ([]T, error) {
	cv, err := m.CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{kind}, true)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", kind, err)
	}
	defer func() { _ = cv.Destroy(context.WithoutCancel(ctx)) }()
	var out []T
	if err := cv.Retrieve(ctx, []string{kind}, props, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", kind, err)
	}
	return out, nil
}

func guestFamily(guestID, fullName string) string {
	name := strings.ToLower(guestID + " " + fullName)
	switch {
	case strings.Contains(name, "windows") && strings.Contains(name, "server"), strings.Contains(name, "winnet"), strings.Contains(name, "windows20"):
		return "windows_server"
	case strings.Contains(name, "win"):
		return "windows"
	case strings.Contains(name, "linux"), strings.Contains(name, "ubuntu"), strings.Contains(name, "rhel"),
		strings.Contains(name, "centos"), strings.Contains(name, "debian"), strings.Contains(name, "sles"):
		return "linux"
	case strings.Contains(name, "darwin"):
		return "macos"
	}
	return "unknown"
}

func portgroupContext(name string) string { return "portgroup:" + name }
