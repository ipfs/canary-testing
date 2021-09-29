package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archiver"
	"github.com/testground/testground/pkg/api"
	"github.com/testground/testground/pkg/logging"
	"github.com/testground/testground/pkg/rpc"
)

func (d *Daemon) buildHandler(engine api.Engine) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ruid := r.Header.Get("X-Request-ID")
		log := logging.S().With("req_id", ruid)

		log.Infow("handle request", "command", "build")
		defer log.Infow("request handled", "command", "build")

		tgw := rpc.NewOutputWriter(w, r)

		// Create a packing directory under the workdir.
		dir := filepath.Join(engine.EnvConfig().Dirs().Work(), "requests", ruid)
		if err := os.MkdirAll(dir, 0755); err != nil {
			tgw.WriteError("failed to create temp directory to unpack request", "err", err)
			return
		}

		var request *api.BuildRequest
		sources, err := consumeRunBuildRequest(r, &request, dir)
		if err != nil {
			tgw.WriteError("failed to consume request", "err", err)
			return
		}

		if sources == nil || sources.PlanDir == "" {
			tgw.WriteError("bad request", "err", errors.New("plan directory not present"))
			return
		}

		id, err := engine.QueueBuild(request, sources)
		if err != nil {
			tgw.WriteError(fmt.Sprintf("engine build error: %s", err))
			return
		}

		tgw.WriteResult(id)
	}
}

func (d *Daemon) buildPurgeHandler(engine api.Engine) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		log := logging.S().With("req_id", r.Header.Get("X-Request-ID"))

		log.Debugw("handle request", "command", "build/purge")
		defer log.Debugw("request handled", "command", "build/purge")

		tgw := rpc.NewOutputWriter(w, r)

		var req api.BuildPurgeRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			tgw.WriteError("build parge json decode", "err", err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err = engine.DoBuildPurge(r.Context(), req.Builder, req.Testplan, tgw)
		if err != nil {
			tgw.WriteError("build purge error", "err", err.Error())
			return
		}

		tgw.WriteResult("build purge succeeded")
	}
}

func consumeRunBuildRequest(r *http.Request, body interface{}, dir string) (*api.UnpackedSources, error) {
	var (
		p   *multipart.Part
		err error
	)

	if r.Body == nil {
		return nil, fmt.Errorf("failed to parse request: nil body")
	}

	defer r.Body.Close()

	// Validate the incoming multipart request.
	ct, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	if !strings.HasPrefix(ct, "multipart/") {
		ct := r.Header.Get("Content-Type")
		return nil, fmt.Errorf("not a multipart request; Content-Type: %s", ct)
	}

	rd := multipart.NewReader(r.Body, params["boundary"])

	// 1. Read and decode the request payload.
	if p, err = rd.NextPart(); err != nil {
		return nil, fmt.Errorf("unexpected error when reading composition: %w", err)
	}
	if err = json.NewDecoder(p).Decode(body); err != nil {
		return nil, fmt.Errorf("failed to json decode request body: %w", err)
	}

	var unpacked *api.UnpackedSources

Outer:
	for {
		switch p, err = rd.NextPart(); err {
		case io.EOF:
			// we're done.
			break Outer
		case nil:
			if unpacked == nil {
				unpacked = new(api.UnpackedSources)
				unpacked.BaseDir = dir
			}

			var (
				filename = p.FileName() // can be sdk.zip or extra.zip
				kind     = strings.TrimSuffix(filename, ".zip")
			)

			// Read the archive.
			targetzip, err := os.Create(filepath.Join(dir, filename))
			if err != nil {
				return nil, fmt.Errorf("failed to create file for %s: %w", kind, err)
			}
			if _, err = io.Copy(targetzip, p); err != nil {
				return nil, fmt.Errorf("unexpected error when copying %s: %w", kind, err)
			}

			// Inflate the archive.
			destdir := filepath.Join(dir, kind)
			if err := os.Mkdir(destdir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory for sdk: %w", err)
			}
			logging.S().Infof("extracting %s to %s", filename, destdir)
			if err := archiver.NewZip().Unarchive(targetzip.Name(), destdir); err != nil {
				return nil, fmt.Errorf("failed to decompress sdk: %w", err)
			}

			// Set the right directory.
			switch kind {
			case "sdk":
				unpacked.SDKDir = destdir
			case "extra":
				unpacked.ExtraDir = destdir
			case "plan":
				unpacked.PlanDir = destdir
			}
		default:
			// an error occurred.
			return nil, fmt.Errorf("unexpected error when reading optional parts: %w", err)
		}
	}

	return unpacked, nil
}
