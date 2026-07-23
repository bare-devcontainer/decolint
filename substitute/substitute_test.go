package substitute

import (
	"encoding/json/v2"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/tailscale/hujson"
)

func TestApply(t *testing.T) {
	t.Parallel()

	ctx := Context{
		LocalEnv:             map[string]string{"HOME": "/home/user", "EMPTY": ""},
		LocalWorkspaceFolder: "/projects/app",
	}

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			"defined localEnv",
			`{"remoteEnv": {"HOME2": "${localEnv:HOME}"}}`,
			`{"remoteEnv": {"HOME2": "/home/user"}}`,
		},
		{
			"undefined localEnv",
			`{"containerEnv": {"A": "x${localEnv:UNSET}y"}}`,
			`{"containerEnv": {"A": "xy"}}`,
		},
		{
			"defined but empty localEnv wins over default",
			`{"image": "${localEnv:EMPTY:fallback}"}`,
			`{"image": ""}`,
		},
		{
			"undefined localEnv with default",
			`{"image": "${localEnv:BASE:ubuntu}"}`,
			`{"image": "ubuntu"}`,
		},
		{
			"default keeps only its own segment",
			`{"image": "${localEnv:BASE:a:b}"}`,
			`{"image": "a"}`,
		},
		{
			"env alias",
			`{"image": "${env:HOME}"}`,
			`{"image": "/home/user"}`,
		},
		{
			"bare localEnv",
			`{"image": "a${localEnv}b"}`,
			`{"image": "ab"}`,
		},
		{
			"empty variable",
			`{"image": "a${}b"}`,
			`{"image": "ab"}`,
		},
		{
			"unknown variable",
			`{"image": "a${templateOption:foo}b"}`,
			`{"image": "ab"}`,
		},
		{
			"containerEnv is not resolvable",
			`{"remoteEnv": {"PATH": "${containerEnv:PATH}:/opt/bin"}}`,
			`{"remoteEnv": {"PATH": ":/opt/bin"}}`,
		},
		{
			"local workspace folder and basename",
			`{"mounts": ["source=${localWorkspaceFolder},target=/w/${localWorkspaceFolderBasename},type=bind"]}`,
			`{"mounts": ["source=/projects/app,target=/w/app,type=bind"]}`,
		},
		{
			"explicit container workspace folder",
			`{"workspaceFolder": "/srv/code", "postCreateCommand": "ls ${containerWorkspaceFolder}/${containerWorkspaceFolderBasename}"}`,
			`{"workspaceFolder": "/srv/code", "postCreateCommand": "ls /srv/code/code"}`,
		},
		{
			"container workspace folder default",
			`{"image": "img:1", "postCreateCommand": "ls ${containerWorkspaceFolder}"}`,
			`{"image": "img:1", "postCreateCommand": "ls /workspaces/app"}`,
		},
		{
			"compose container workspace folder default",
			`{"dockerComposeFile": "compose.yml", "postCreateCommand": "ls ${containerWorkspaceFolder}${containerWorkspaceFolderBasename}"}`,
			`{"dockerComposeFile": "compose.yml", "postCreateCommand": "ls /"}`,
		},
		{
			"workspaceFolder is itself resolved",
			`{"workspaceFolder": "/srv/${localEnv:HOME:x}", "postCreateCommand": "ls ${containerWorkspaceFolder}"}`,
			`{"workspaceFolder": "/srv//home/user", "postCreateCommand": "ls /srv//home/user"}`,
		},
		{
			// A self-reference resolves to the raw value, as the reference implementation does,
			// and the embedded ${...} text survives since replaced text is never re-scanned.
			"workspaceFolder self-reference",
			`{"workspaceFolder": "${containerWorkspaceFolder}/sub", "postCreateCommand": "ls ${containerWorkspaceFolder}"}`,
			`{"workspaceFolder": "${containerWorkspaceFolder}/sub/sub/sub", "postCreateCommand": "ls ${containerWorkspaceFolder}/sub/sub"}`,
		},
		{
			"devcontainerId",
			`{"name": "dc-${devcontainerId}"}`,
			`{"name": "dc-` + DevcontainerID + `"}`,
		},
		{
			"multiple variables in one string",
			`{"image": "${localEnv:HOME}-${devcontainerId}-${localEnv:UNSET}"}`,
			`{"image": "/home/user-` + DevcontainerID + `-"}`,
		},
		{
			// The lazy pattern matches "${a${b}" (an unknown variable) and leaves the trailing
			// brace alone, exactly as the reference implementation's regex does.
			"nested braces",
			`{"image": "${a${localEnv:HOME}}"}`,
			`{"image": "}"}`,
		},
		{
			"object member names are not substituted",
			`{"containerEnv": {"${localEnv:HOME}": "${localEnv:HOME}"}}`,
			`{"containerEnv": {"${localEnv:HOME}": "/home/user"}}`,
		},
		{
			"non-string values untouched",
			`{"privileged": true, "appPort": 8080, "features": null}`,
			`{"privileged": true, "appPort": 8080, "features": null}`,
		},
		{
			"non-string workspaceFolder falls back to the default",
			`{"workspaceFolder": 42, "postCreateCommand": "ls ${containerWorkspaceFolder}"}`,
			`{"workspaceFolder": 42, "postCreateCommand": "ls /workspaces/app"}`,
		},
		{
			"non-object root untouched",
			`["${localEnv:HOME}"]`,
			`["/home/user"]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree, err := hujson.Parse([]byte(tt.src))
			if err != nil {
				t.Fatalf("parse src: %v", err)
			}
			Apply(ctx, &tree)
			tree.Standardize()
			var got, want any
			if err := json.Unmarshal(tree.Pack(), &got); err != nil {
				t.Fatalf("unmarshal substituted tree: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
				t.Fatalf("unmarshal want: %v", err)
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("substituted configuration mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestApply_OffsetsPreserved(t *testing.T) {
	t.Parallel()

	src := `{"name": "n", "image": "${localEnv:BASE:ubuntu}:latest"}`
	tree, err := hujson.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse src: %v", err)
	}
	image := tree.Find("/image")
	wantStart, wantEnd := image.StartOffset, image.EndOffset

	Apply(Context{}, &tree)

	if lit, ok := image.Value.(hujson.Literal); !ok || lit.String() != "ubuntu:latest" {
		t.Fatalf("image = %v, want substituted \"ubuntu:latest\"", image.Value)
	}
	if image.StartOffset != wantStart || image.EndOffset != wantEnd {
		t.Errorf("image offsets = [%d, %d], want original [%d, %d]",
			image.StartOffset, image.EndOffset, wantStart, wantEnd)
	}
}

func TestDevcontainerID_Format(t *testing.T) {
	t.Parallel()

	if !regexp.MustCompile(`^[0-9a-v]{52}$`).MatchString(DevcontainerID) {
		t.Errorf("DevcontainerID = %q, want 52 base-32 chars", DevcontainerID)
	}
}
