package main

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fmt"

	"github.com/alexflint/go-arg"
	"github.com/google/go-containerregistry/authn"
	"github.com/google/go-containerregistry/name"
	v1 "github.com/google/go-containerregistry/v1"
	"github.com/google/go-containerregistry/v1/mutate"
	"github.com/google/go-containerregistry/v1/remote"
	"github.com/google/go-containerregistry/v1/tarball"
	"github.com/pkg/errors"
)

type params struct {
	Target    string   `arg:"-t,required" help:"destination image GCR-URL"`
	BaseImage string   `arg:"-b,required" help:"base image GCR-URL"`
	Files     []string `arg:"-f,separate" help:"Specify a file to add"`
	Env       []string `arg:"-e,separate" help:"Environment Variables"`
	Cmd       string   `arg:"-c" help:"Command to run when starting the container"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
		os.Exit(1)
	}
}

func run() error {
	p := &params{}
	arg.MustParse(p)

	fmt.Println("Checking base image...")

	baseImage, repository, err := getImage(p.BaseImage)
	if err != nil {
		return err
	}

	fmt.Println("Building new image...")

	finalImage, err := buildNewImage(p.Files, p.Env, p.Cmd, baseImage)
	if err != nil {
		return err
	}

	fmt.Println("Pushing...")

	if err := pushImage(finalImage, []name.Repository{
		repository,
	}, p.Target); err != nil {
		return err
	}

	fmt.Println("Done.")

	return nil
}

func buildNewImage(files, env []string, cmd string, baseImage v1.Image) (v1.Image, error) {
	image, err := applyConfig(baseImage, env, cmd)
	if err != nil {
		return nil, errors.Wrap(err, "applying config")
	}

	image, err = addNewLayerFromFiles(image, files)
	if err != nil {
		return nil, errors.Wrap(err, "adding layer")
	}

	return image, nil
}

func applyConfig(image v1.Image, env []string, cmd string) (v1.Image, error) {
	imageConfig, err := image.ConfigFile()
	if err != nil {
		return nil, errors.Wrap(err, "creating config file")
	}

	imageConfig.Config.Env = env
	imageConfig.Config.Cmd = []string{cmd}

	newImage, err := mutate.Config(image, imageConfig.Config)
	if err != nil {
		return nil, errors.Wrap(err, "applying new config")
	}
	newImage, err = mutate.CreatedAt(newImage, v1.Time{Time: time.Now()})
	if err != nil {
		return nil, errors.Wrap(err, "setting created-at timestamp")
	}

	return newImage, nil
}

func addNewLayerFromFiles(image v1.Image, files []string) (v1.Image, error) {
	// the .tar file from the passed files will be our new layer
	bb := bytes.Buffer{}
	if err := createTarFile(files, &bb); err != nil {
		return nil, errors.Wrap(err, "creating tar archive")
	}

	// wrapper for LayerFromOpener
	opener := func() (io.ReadCloser, error) {
		return ioutil.NopCloser(bytes.NewReader(bb.Bytes())), nil
	}

	// create new layer from our .tar
	l, err := tarball.LayerFromOpener(opener)
	if err != nil {
		return nil, errors.Wrap(err, "creating layer")
	}

	image, err = mutate.AppendLayers(image, l)
	if err != nil {
		return nil, errors.Wrap(err, "appending Layer")
	}

	return image, nil
}

func pushImage(image v1.Image, repositories []name.Repository, destURL string) error {
	destRef, err := name.ParseReference(destURL, name.WeakValidation)
	if err != nil {
		return errors.Wrapf(err, "parsing destination URL (%s)", destURL)
	}

	pushAuth, err := authn.DefaultKeychain.Resolve(destRef.Context().Registry)
	if err != nil {
		return errors.Wrapf(err, "authenticating target (%s)", destURL)
	}

	wo := remote.WriteOptions{}
	wo.MountPaths = repositories

	return remote.Write(destRef, image, pushAuth, http.DefaultTransport, wo)
}

func getImage(sourceURL string) (v1.Image, name.Repository, error) {
	ref, err := parseImageURL(sourceURL)
	if err != nil {
		return nil, name.Repository{}, errors.Wrap(err, "parsing source URL")
	}

	auth, err := authn.DefaultKeychain.Resolve(ref.Context().Registry)
	if err != nil {
		return nil, name.Repository{}, errors.Wrap(err, "authenticating")
	}

	img, err := remote.Image(ref, auth, http.DefaultTransport)
	if err != nil {
		return nil, name.Repository{}, errors.Wrap(err, "fetching")
	}

	return img, ref.Context(), nil
}

func parseImageURL(url string) (name.Reference, error) {
	ref, err := name.ParseReference(url, name.WeakValidation)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing url (%s)", url)
	}
	return ref, nil
}

// from https://github.com/verybluebot/tarinator-go/blob/master/tarinator.go
func createTarFile(paths []string, writer io.Writer) error {
	tw := tar.NewWriter(writer)
	defer tw.Close()

	for _, i := range paths {
		if err := tarwalk(i, tw); err != nil {
			return err
		}
	}

	return nil
}

func tarwalk(source string, tw *tar.Writer) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	return filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}

			if baseDir != "" {
				header.Name = filepath.ToSlash(filepath.Join(baseDir, strings.TrimPrefix(path, source)))
			}

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			if !info.Mode().IsRegular() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(tw, file)
			return err
		})
}
