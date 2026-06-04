package chartsource

import "testing"

func TestDetect(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		want    Source
		wantErr bool
	}{
		{
			name: "local relative dir",
			ref:  "./mychart",
			want: Source{Kind: KindLocal, Location: "./mychart"},
		},
		{
			name: "local absolute dir",
			ref:  "/abs/path/chart",
			want: Source{Kind: KindLocal, Location: "/abs/path/chart"},
		},
		{
			name: "local packaged tgz",
			ref:  "chart-1.2.3.tgz",
			want: Source{Kind: KindLocal, Location: "chart-1.2.3.tgz"},
		},
		{
			name: "oci with version",
			ref:  "oci://registry.example.com/charts/app:1.2.3",
			want: Source{Kind: KindOCI, Location: "oci://registry.example.com/charts/app", Version: "1.2.3"},
		},
		{
			name: "oci without version",
			ref:  "oci://registry.example.com/charts/app",
			want: Source{Kind: KindOCI, Location: "oci://registry.example.com/charts/app"},
		},
		{
			name: "git+https with ref and path",
			ref:  "git+https://github.com/org/repo.git#v1.4.0?path=charts/app",
			want: Source{Kind: KindGit, Location: "https://github.com/org/repo.git", Ref: "v1.4.0", SubPath: "charts/app"},
		},
		{
			name: "git+https with path then ref order-insensitive query",
			ref:  "git+https://github.com/org/repo.git?path=charts/app#v1.4.0",
			want: Source{Kind: KindGit, Location: "https://github.com/org/repo.git", Ref: "v1.4.0", SubPath: "charts/app"},
		},
		{
			name: "bare .git https url",
			ref:  "https://github.com/org/repo.git",
			want: Source{Kind: KindGit, Location: "https://github.com/org/repo.git"},
		},
		{
			name: "scp-style git address",
			ref:  "git@github.com:org/repo.git",
			want: Source{Kind: KindGit, Location: "git@github.com:org/repo.git"},
		},
		{
			name: "git+ssh url",
			ref:  "git+ssh://git@github.com/org/repo.git#main",
			want: Source{Kind: KindGit, Location: "ssh://git@github.com/org/repo.git", Ref: "main"},
		},
		{
			name: "http archive tgz",
			ref:  "https://example.com/charts/app-1.2.3.tgz",
			want: Source{Kind: KindHTTPArchive, Location: "https://example.com/charts/app-1.2.3.tgz"},
		},
		{
			name: "http archive tar.gz",
			ref:  "https://example.com/charts/app.tar.gz",
			want: Source{Kind: KindHTTPArchive, Location: "https://example.com/charts/app.tar.gz"},
		},
		{
			name: "plain helm repo url",
			ref:  "https://charts.example.com",
			want: Source{Kind: KindHelmRepo, Location: "https://charts.example.com"},
		},
		{
			name:    "git with unknown query key",
			ref:     "git+https://github.com/org/repo.git?branch=main",
			wantErr: true,
		},
		{
			name:    "empty ref",
			ref:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Detect(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Detect(%q) = %+v, want error", tt.ref, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Detect(%q) unexpected error: %v", tt.ref, err)
			}
			if got != tt.want {
				t.Fatalf("Detect(%q) = %+v, want %+v", tt.ref, got, tt.want)
			}
		})
	}
}
