package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/script/v1"
)

// File extensions map Terraform file names to Apps Script file types.
var fileTypes = map[string]string{
	".gs":   "SERVER_JS",
	".html": "HTML",
	".json": "JSON",
}

// toAPIFile converts a file name like "lib/util.gs" to an Apps Script file.
func toAPIFile(name, source string) (*script.File, error) {
	for ext, typ := range fileTypes {
		if base, ok := strings.CutSuffix(name, ext); ok && base != "" {
			return &script.File{Name: base, Type: typ, Source: source}, nil
		}
	}
	return nil, fmt.Errorf("unsupported file %q: must end with .gs, .html or .json", name)
}

// fromAPIFile returns the Terraform file name for an Apps Script file.
func fromAPIFile(f *script.File) (string, error) {
	for ext, typ := range fileTypes {
		if f.Type == typ {
			return f.Name + ext, nil
		}
	}
	return "", fmt.Errorf("unsupported file type %q for %q", f.Type, f.Name)
}

// jsonEqual reports whether a and b are semantically equal JSON documents.
func jsonEqual(a, b string) bool {
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

func isNotFound(err error) bool {
	var gerr *googleapi.Error
	return errors.As(err, &gerr) && gerr.Code == http.StatusNotFound
}

// pollInterval is the delay between checks in waitForStable.
var pollInterval = 2 * time.Second

// waitForStable polls cond until it holds for several consecutive checks, or
// returns an error or times out. Apps Script API reads are eventually
// consistent and may flap between old and new values for a while.
func waitForStable(ctx context.Context, cond func() (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for n := 0; n < 5; {
		ok, err := cond()
		if err != nil {
			return err
		}
		if ok {
			n++
		} else {
			n = 0
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for changes to propagate: %w", ctx.Err())
		case <-time.After(pollInterval):
		}
	}
	return nil
}

func configureClient(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *client {
	if req.ProviderData == nil {
		return nil
	}
	c, ok := req.ProviderData.(*client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
	}
	return c
}
