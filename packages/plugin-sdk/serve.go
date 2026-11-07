package kyoku

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	api "github.com/zertex-labs/kyoku-hub/packages/plugin-api"
)

// Serve is the whole runtime: describe, one payload in, one response out.
func Serve(p Plugin) {
	if len(os.Args) > 1 && os.Args[1] == "--describe" {
		describe(p)
		return
	}
	os.Exit(run(p, os.Stdin, os.Stdout, os.Stderr))
}

func describe(p Plugin) {
	d := api.Describe{APIVersion: api.APIVersion, Name: p.Name, Version: p.Version, Hooks: p.hooks()}
	if err := json.NewEncoder(os.Stdout).Encode(d); err != nil {
		fmt.Fprintf(os.Stderr, "describe: %v\n", err)
		os.Exit(api.ExitError)
	}
	os.Exit(api.ExitOK)
}

// Dispatch runs the handler for a payload and returns its answer. Serve wraps
// this; a plugin's own go test calls it directly, so tests exercise the same
// decoding and the same Event a real run gets.
func Dispatch(ctx context.Context, p Plugin, payload api.Payload) (api.Response, error) {
	handler, ok := p.handlerFor(payload.Hook)
	if !ok {
		return api.Response{}, fmt.Errorf("%s: hook %q is not implemented by this plugin", p.Name, payload.Hook)
	}
	ev := &Event{
		Hook:   payload.Hook,
		Plugin: payload.Plugin,
		App:    payload.App,
		Album:  payload.Album,
		Track:  payload.Track,
		Files:  payload.Files,
		config: payload.Config,
	}
	if err := handler(ctx, ev); err != nil {
		return ev.response, err
	}
	return ev.response, nil
}

// run is Serve without the process boundary: payload in, response out, exit code.
func run(p Plugin, stdin *os.File, stdout, stderr *os.File) int {
	var payload api.Payload
	if err := json.NewDecoder(stdin).Decode(&payload); err != nil {
		fmt.Fprintf(stderr, "%s: reading payload: %v\n", p.Name, err)
		return api.ExitError
	}

	resp, err := Dispatch(context.Background(), p, payload)

	switch {
	case err == nil:
	case isSkipReason(err):
		reason, _ := isSkip(err)
		fmt.Fprintf(stderr, "%s: skipped: %s\n", p.Name, reason)
		return api.ExitSkip
	default:
		fmt.Fprintf(stderr, "%s: %v\n", p.Name, err)
		return api.ExitError
	}

	if resp.Message != "" || len(resp.Files) > 0 {
		if encErr := json.NewEncoder(stdout).Encode(resp); encErr != nil {
			fmt.Fprintf(stderr, "%s: writing response: %v\n", p.Name, encErr)
			return api.ExitError
		}
	}
	return api.ExitOK
}

func isSkipReason(err error) bool {
	_, ok := isSkip(err)
	return ok
}
