// SPDX-License-Identifier: Apache-2.0
package zap

import "testing"

func TestConfigRoundTrip(t *testing.T) {
	input := `schema: 1
project:
  environment: pico-sdk
  target: app
  board: pico2_w
  build_dir: build
  deps_dir: deps
  adapters_dir: adapters
dependencies:
  zapee:
    type: git
    uri: https://example.invalid/zap-ee.git
    version: v1.2.3
    override_var: ZAPEE_SOURCE
    zephyr_module: true
    components:
      - zap::pico_ztm_pwm
`
	c, err := ParseConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	if c.Project.Environment != "pico-sdk" || c.Dependencies["zapee"].Version != "v1.2.3" {
		t.Fatalf("unexpected parse: %#v", c)
	}
	c2, err := ParseConfig(FormatConfig(c))
	if err != nil {
		t.Fatal(err)
	}
	if got := c2.Dependencies["zapee"].Components[0]; got != "zap::pico_ztm_pwm" {
		t.Fatalf("component = %q", got)
	}
}

func TestPackageResolve(t *testing.T) {
	m, err := ParsePackageManifest(`schema: 1
package:
  name: Demo
  paths:
    - cmake
components:
  zap::i2c:
    kind: transport
    paths:
      - platform
  zap::chip:
    kind: driver
    depends:
      - zap::i2c
    paths:
      - drivers/chip
  zap::board:
    kind: module
    pico: zap::pico_board
    depends:
      - zap::chip
    paths:
      - modules/board
`)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := resolveSparsePaths(m, []string{"zap::board"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"cmake": true, "platform": true, "drivers/chip": true, "modules/board": true}
	for _, p := range paths {
		delete(want, p)
	}
	if len(want) != 0 {
		t.Fatalf("missing paths: %#v (got %#v)", want, paths)
	}
}

func TestQuotedWindowsPathRoundTrip(t *testing.T) {
	c := &Config{Schema: 1, Project: ProjectConfig{Environment: "generic", Target: "app", BuildDir: "build", DepsDir: `V:\\Deps`, AdaptersDir: "adapters"}, Dependencies: map[string]*DependencyConfig{}, DependencyOrder: nil}
	text := FormatConfig(c)
	r, err := ParseConfig(text)
	if err != nil {
		t.Fatal(err)
	}
	if r.Project.DepsDir != c.Project.DepsDir {
		t.Fatalf("deps dir round trip: %q != %q\n%s", r.Project.DepsDir, c.Project.DepsDir, text)
	}
}

func TestRejectsRemoteCMakeTargetInjection(t *testing.T) {
	_, err := ParsePackageManifest(`schema: 1
package:
  name: Demo
  paths:
    - cmake
components:
  zap::board:
    kind: module
    pico: "foo)\nexecute_process(COMMAND bad)\n#"
    paths:
      - modules/board
`)
	if err == nil {
		t.Fatal("expected malicious remote CMake target to be rejected")
	}
}

func TestURLDependencyRequiresSHA256(t *testing.T) {
	_, err := ParseConfig(`schema: 2
project:
  environment: generic
  target: app
  build_dir: build
  deps_dir: deps
  adapters_dir: adapters
dependencies:
  archive:
    type: url
    uri: https://example.invalid/archive.tar.gz
    components:
`)
	if err == nil {
		t.Fatal("expected URL dependency without SHA256 to be rejected")
	}
}

func TestRejectsUnsafeComponentInProjectManifest(t *testing.T) {
	_, err := ParseConfig(`schema: 2
project:
  environment: generic
  target: app
  build_dir: build
  deps_dir: deps
  adapters_dir: adapters
dependencies:
  demo:
    type: git
    uri: https://example.invalid/demo.git
    version: v1.0.0
    commit: 0123456789abcdef0123456789abcdef01234567
    components:
      - "foo);execute_process(COMMAND bad)"
`)
	if err == nil {
		t.Fatal("expected unsafe CMake component name to be rejected")
	}
}

func TestBuildConfigRoundTrip(t *testing.T) {
	input := `schema: 3
project:
  environment: pico-sdk
  target: Hub
  board: pico2_w
  build_dir: build
  deps_dir: deps
  adapters_dir: adapters
build:
  configuration: Release
  generator: Ninja
  cmake:
    HUB_WIFI_SSID: env:ZAP_HUB_WIFI_SSID
    HUB_WIFI_PASSWORD: env:ZAP_HUB_WIFI_PASSWORD
dependencies:
`
	c, err := ParseConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	if c.Build.Configuration != "Release" || c.Build.Generator != "Ninja" {
		t.Fatalf("unexpected build config: %#v", c.Build)
	}
	if got := c.Build.CMake["HUB_WIFI_SSID"]; got != "env:ZAP_HUB_WIFI_SSID" {
		t.Fatalf("wifi mapping = %q", got)
	}
	r, err := ParseConfig(FormatConfig(c))
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Build.CMake["HUB_WIFI_PASSWORD"]; got != "env:ZAP_HUB_WIFI_PASSWORD" {
		t.Fatalf("password mapping = %q", got)
	}
}

func TestUploadConfigRoundTrip(t *testing.T) {
	input := `schema: 4
project:
  environment: pico-sdk
  target: Hub
  board: pico2_w
  build_dir: build
  deps_dir: deps
  adapters_dir: adapters
build:
  configuration: Release
  generator: Ninja
upload:
  default: auto
  order:
    - bootsel
    - openocd
  methods:
    bootsel:
      type: volume-copy
      artifact: "{build_dir}/{target}.uf2"
      volume_labels:
        - RPI-RP2
        - RP2350
    openocd:
      type: command
      artifact: "{build_dir}/{target}.elf"
      executable: openocd
      search:
        - PATH
        - "{pico_home}/openocd"
      variables:
        probe_serial: env:ZAP_PROBE_SERIAL
        adapter_speed_khz: 5000
      find:
        scripts:
          root: "{pico_home}/openocd"
          file: rp2350.cfg
          parent: 2
      args:
        - -s
        - "{scripts}"
        - -c
        - "adapter speed {adapter_speed_khz}"
        - -c
        - "program \"{artifact}\" verify reset exit"
      optional_args:
        probe_serial:
          - -c
          - "adapter serial {probe_serial}"
dependencies:
`
	c, err := ParseConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	if c.Upload.Default != "auto" || len(c.Upload.Order) != 2 {
		t.Fatalf("unexpected upload config: %#v", c.Upload)
	}
	m := c.Upload.Methods["openocd"]
	if m == nil || m.Find["scripts"].Parent != 2 || m.Variables["adapter_speed_khz"] != "5000" {
		t.Fatalf("unexpected openocd config: %#v", m)
	}
	r, err := ParseConfig(FormatConfig(c))
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Upload.Methods["openocd"].OptionalArgs["probe_serial"]; len(got) != 2 || got[1] != "adapter serial {probe_serial}" {
		t.Fatalf("optional args round trip = %#v", got)
	}
}
