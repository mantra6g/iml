package p4compile

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	oldproto "github.com/golang/protobuf/proto"
	p4configv1 "github.com/p4lang/p4runtime/go/p4/config/v1"
)

// Result holds the output of a successful P4 compilation.
type Result struct {
	DeviceConfig []byte
	P4Info       *p4configv1.P4Info
}

// CompileFromURL downloads a .p4 source file from fileURL and compiles it with
// p4c for the bmv2/v1model target.
func CompileFromURL(ctx context.Context, fileURL string) (*Result, error) {
	tmpDir, err := os.MkdirTemp("", "p4compile-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inputPath := tmpDir + "/input.p4"

	if strings.HasPrefix(fileURL, "oci://") {
		err = downloadOCIArtifact(strings.TrimPrefix(fileURL, "oci://"), "input.p4", inputPath)
	} else {
		err = downloadFile(fileURL, inputPath)
	}
	if err != nil {
		return nil, err
	}
	return compileP4File(ctx, inputPath, tmpDir)
}


func downloadOCIArtifact(imageRef, targetFile, destPath string) error {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return err
	}

	img, err := remote.Image(ref)
	if err != nil {
		return err
	}

	fs := mutate.Extract(img)
	defer fs.Close()

	tr := tar.NewReader(fs)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("File not found in artifact")
		}
		if err != nil {
			return err
		}

		if header.Name != targetFile {
			continue
		}

		f, err := os.Create(destPath)
		if err != nil {
			return fmt.Errorf("creating file: %w", err)
		}
		defer f.Close()

		if _, err := io.Copy(f, tr); err != nil {
			return fmt.Errorf("writing file: %w", err)
		}
		return nil
	}
}

func downloadFile(fileURL, destPath string) error {
	resp, err := http.Get(fileURL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("downloading %q: %w", fileURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("downloading %q: HTTP %d", fileURL, resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	return nil
}

func compileP4File(ctx context.Context, inputPath, outDir string) (*Result, error) {
	p4infoPath := outDir + "/p4info.bin"

	compileCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(compileCtx, "p4c",
	"--target", "bmv2",
	"--arch", "v1model",
	"--p4runtime-files", p4infoPath,
	"--p4runtime-format", "binary",
	"-o", outDir,
	inputPath,
)
if out, err := cmd.CombinedOutput(); err != nil {
	return nil, fmt.Errorf("p4c: %s", string(out))
}

jsonBytes, err := os.ReadFile(outDir + "/input.json")
if err != nil {
	return nil, fmt.Errorf("reading compiled JSON: %w", err)
}

p4infoBytes, err := os.ReadFile(p4infoPath)
if err != nil {
	return nil, fmt.Errorf("reading p4info: %w", err)
}

var p4info p4configv1.P4Info
if err := oldproto.Unmarshal(p4infoBytes, &p4info); err != nil {
	return nil, fmt.Errorf("parsing p4info: %w", err)
}

return &Result{
	DeviceConfig: jsonBytes,
	P4Info:       &p4info,
}, nil
}
