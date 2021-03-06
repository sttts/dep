// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dep

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/dep/internal/gps"
	"github.com/golang/dep/internal/test"
)

func TestReadManifest(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	mf := h.GetTestFile("manifest/golden.toml")
	defer mf.Close()
	got, _, err := readManifest(mf)
	if err != nil {
		t.Fatalf("Should have read Manifest correctly, but got err %q", err)
	}

	c, _ := gps.NewSemverConstraint(">=0.12.0, <1.0.0")
	want := Manifest{
		Dependencies: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/golang/dep/internal/gps"): {
				Constraint: c,
			},
			gps.ProjectRoot("github.com/babble/brook"): {
				Constraint: gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb"),
			},
		},
		Ovr: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/golang/dep/internal/gps"): {
				Source:     "https://github.com/golang/dep/internal/gps",
				Constraint: gps.NewBranch("master"),
			},
		},
		Ignored: []string{"github.com/foo/bar"},
	}

	if !reflect.DeepEqual(got.Dependencies, want.Dependencies) {
		t.Error("Valid manifest's dependencies did not parse as expected")
	}
	if !reflect.DeepEqual(got.Ovr, want.Ovr) {
		t.Error("Valid manifest's overrides did not parse as expected")
	}
	if !reflect.DeepEqual(got.Ignored, want.Ignored) {
		t.Error("Valid manifest's ignored did not parse as expected")
	}
}

func TestWriteManifest(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()

	golden := "manifest/golden.toml"
	want := h.GetTestFileString(golden)
	c, _ := gps.NewSemverConstraint(">=0.12.0, <1.0.0")
	m := &Manifest{
		Dependencies: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/golang/dep/internal/gps"): {
				Constraint: c,
			},
			gps.ProjectRoot("github.com/babble/brook"): {
				Constraint: gps.Revision("d05d5aca9f895d19e9265839bffeadd74a2d2ecb"),
			},
		},
		Ovr: map[gps.ProjectRoot]gps.ProjectProperties{
			gps.ProjectRoot("github.com/golang/dep/internal/gps"): {
				Source:     "https://github.com/golang/dep/internal/gps",
				Constraint: gps.NewBranch("master"),
			},
		},
		Ignored: []string{"github.com/foo/bar"},
	}

	got, err := m.MarshalTOML()
	if err != nil {
		t.Fatalf("Error while marshaling valid manifest to TOML: %q", err)
	}

	if string(got) != want {
		if *test.UpdateGolden {
			if err = h.WriteTestFile(golden, string(got)); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Errorf("Valid manifest did not marshal to TOML as expected:\n\t(GOT): %s\n\t(WNT): %s", string(got), want)
		}
	}
}

func TestReadManifestErrors(t *testing.T) {
	h := test.NewHelper(t)
	defer h.Cleanup()
	var err error

	tests := []struct {
		name string
		file string
	}{
		{"multiple constraints", "manifest/error1.toml"},
		{"multiple dependencies", "manifest/error2.toml"},
	}

	for _, tst := range tests {
		mf := h.GetTestFile(tst.file)
		defer mf.Close()
		_, _, err = readManifest(mf)
		if err == nil {
			t.Errorf("Reading manifest with %s should have caused error, but did not", tst.name)
		} else if !strings.Contains(err.Error(), tst.name) {
			t.Errorf("Unexpected error %q; expected %s error", err, tst.name)
		}
	}
}

func TestValidateManifest(t *testing.T) {
	cases := []struct {
		tomlString string
		want       []error
	}{
		{
			tomlString: `
			[[dependencies]]
			  name = "github.com/foo/bar"
			`,
			want: []error{},
		},
		{
			tomlString: `
			[metadata]
			  authors = "foo"
			  version = "1.0.0"
			`,
			want: []error{},
		},
		{
			tomlString: `
			foo = "some-value"
			version = 14

			[[bar]]
			  author = "xyz"

			[[dependencies]]
			  name = "github.com/foo/bar"
			  version = ""
			`,
			want: []error{
				errors.New("Unknown field in manifest: foo"),
				errors.New("Unknown field in manifest: bar"),
				errors.New("Unknown field in manifest: version"),
			},
		},
		{
			tomlString: `
			metadata = "project-name"

			[[dependencies]]
			  name = "github.com/foo/bar"
			`,
			want: []error{errors.New("metadata should be a TOML table")},
		},
		{
			tomlString: `
			dependencies = "foo"
			overrides = "bar"
			`,
			want: []error{
				errors.New("dependencies should be a TOML array of tables"),
				errors.New("overrides should be a TOML array of tables"),
			},
		},
		{
			tomlString: `
			[[dependencies]]
			  name = "github.com/foo/bar"
			  location = "some-value"
			  link = "some-other-value"
			  metadata = "foo"

			[[overrides]]
			  nick = "foo"
			`,
			want: []error{
				errors.New("Invalid key \"location\" in \"dependencies\""),
				errors.New("Invalid key \"link\" in \"dependencies\""),
				errors.New("Invalid key \"nick\" in \"overrides\""),
				errors.New("metadata in \"dependencies\" should be a TOML table"),
			},
		},
		{
			tomlString: `
			[[dependencies]]
			  name = "github.com/foo/bar"

			  [dependencies.metadata]
			    color = "blue"
			`,
			want: []error{},
		},
	}

	// constains for error
	contains := func(s []error, e error) bool {
		for _, a := range s {
			if a.Error() == e.Error() {
				return true
			}
		}
		return false
	}

	for _, c := range cases {
		errs, err := validateManifest(c.tomlString)
		if err != nil {
			t.Fatal(err)
		}

		// compare length of error slice
		if len(errs) != len(c.want) {
			t.Fatalf("Number of manifest errors are not as expected: \n\t(GOT) %v errors(%v)\n\t(WNT) %v errors(%v).", len(errs), errs, len(c.want), c.want)
		}

		// check if the expected errors exist in actual errors slice
		for _, er := range errs {
			if !contains(c.want, er) {
				t.Fatalf("Manifest errors are not as expected: \n\t(MISSING) %v\n\t(FROM) %v", er, c.want)
			}
		}
	}
}
