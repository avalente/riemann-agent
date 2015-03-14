package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type Ctx struct {
	dir string
}

var ctx Ctx

func tearDown(ctx Ctx) {
	os.RemoveAll(ctx.dir)
}

func createCF(ctx Ctx, content string) string {
	file, err := ioutil.TempFile(ctx.dir, "config")
	if err != nil {
		panic(err)
	}

	defer file.Close()

	file.Write([]byte(content))

	return file.Name()
}

func setUp() Ctx {
	tmpdir, err := ioutil.TempDir("", "test-")
	if err != nil {
		panic(err)
	}

	return Ctx{dir: tmpdir}

}

func TestMain(m *testing.M) {
	ctx = setUp()

	defer tearDown(ctx)

	r := m.Run()
	os.Exit(r)
}

func AssertEqual(m *testing.T, v1 interface{}, v2 interface{}) {
	if v1 != v2 {
		m.Errorf("%s != %s", v1, v2)
	}
}

func checkError(m *testing.T, value error, emsg string) {
	if value == nil {
		m.Error("expected error - nil found")
	}
	re := regexp.MustCompile(strings.ToLower(emsg))
	if re.FindStringIndex(strings.ToLower(value.Error())) == nil {
		m.Errorf("the string %s was not found in the error message (%s)", emsg, value.Error())
	}
}

func checkNoResults(m *testing.T, value *Configuration) {
	if value != nil {
		m.Errorf("Unexpected result found: %+v", *value)
	}
}

func TestNewConfiguration(m *testing.T) {
	nc := NewConfiguration()

	AssertEqual(m, nc.DriversDirectory, "drivers")
	AssertEqual(m, nc.RiemannHost, "localhost:5555")
	AssertEqual(m, nc.RiemannProtocol, "udp")
	AssertEqual(m, nc.PidFile, "")
}

func TestGetConfigurationNotFound(m *testing.T) {
	cfg, err := GetConfiguration(filepath.Join(ctx.dir, "non-existing.json"))

	checkNoResults(m, cfg)
	checkError(m, err, "no such file")

}

func TestGetConfigurationInvalidJson(m *testing.T) {
	fileName := createCF(ctx, "##invalid##")
	cfg, err := GetConfiguration(fileName)
	if cfg != nil || err == nil {
		m.FailNow()
	}
}

func TestGetConfigurationEmptyRiemannProtocol(m *testing.T) {
	file := createCF(ctx, "{\"riemannprotocol\": \"\"}")
	cfg, err := GetConfiguration(file)

	checkNoResults(m, cfg)
	checkError(m, err, "bad.*protocol")
}

func TestGetConfigurationBadRiemannProtocol(m *testing.T) {
	file := createCF(ctx, "{\"riemannprotocol\": \"xxx\"}")
	cfg, err := GetConfiguration(file)

	checkNoResults(m, cfg)
	checkError(m, err, "protocol")
}

func TestGetConfigurationEmptyDriversDir(m *testing.T) {
	file := createCF(ctx, "{\"driversdirectory\": \"\"}")
	cfg, err := GetConfiguration(file)

	checkNoResults(m, cfg)
	checkError(m, err, "empty drivers")
}

func TestGetConfigurationDefaults(m *testing.T) {
	file := createCF(ctx, "{}")
	cfg, err := GetConfiguration(file)

	if err != nil {
		m.Errorf("No errors expected, found %s", err.Error())
	}

	if cfg == nil {
		m.Errorf("No configuration found")
	}

}
