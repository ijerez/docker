package main

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/docker/docker/dockerversion"
	"github.com/docker/docker/pkg/homedir"
	"github.com/docker/docker/pkg/integration/checker"
	"github.com/go-check/check"
)

func (s *DockerSuite) TestConfigHTTPHeader(c *check.C) {
	testRequires(c, UnixCli) // Can't set/unset HOME on windows right now
	// We either need a level of Go that supports Unsetenv (for cases
	// when HOME/USERPROFILE isn't set), or we need to be able to use
	// os/user but user.Current() only works if we aren't statically compiling

	var headers map[string][]string

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			headers = r.Header
		}))
	defer server.Close()

	homeKey := homedir.Key()
	homeVal := homedir.Get()
	tmpDir, err := ioutil.TempDir("", "fake-home")
	c.Assert(err, checker.IsNil)
	defer os.RemoveAll(tmpDir)

	dotDocker := filepath.Join(tmpDir, ".docker")
	os.Mkdir(dotDocker, 0600)
	tmpCfg := filepath.Join(dotDocker, "config.json")

	defer func() { os.Setenv(homeKey, homeVal) }()
	os.Setenv(homeKey, tmpDir)

	data := `{
		"HttpHeaders": { "MyHeader": "MyValue" }
	}`

	err = ioutil.WriteFile(tmpCfg, []byte(data), 0600)
	c.Assert(err, checker.IsNil)

	cmd := exec.Command(dockerBinary, "-H="+server.URL[7:], "ps")
	out, _, _ := runCommandWithOutput(cmd)

	c.Assert(headers["User-Agent"], checker.NotNil, check.Commentf("Missing User-Agent"))

	c.Assert(headers["User-Agent"][0], checker.Equals, "Docker-Client/"+dockerversion.Version+" ("+runtime.GOOS+")", check.Commentf("Badly formatted User-Agent,out:%v", out))

	c.Assert(headers["Myheader"], checker.NotNil)
	c.Assert(headers["Myheader"][0], checker.Equals, "MyValue", check.Commentf("Missing/bad header,out:%v", out))

}

func (s *DockerSuite) TestConfigDir(c *check.C) {
	cDir, err := ioutil.TempDir("", "fake-home")
	c.Assert(err, checker.IsNil)
	defer os.RemoveAll(cDir)

	// First make sure pointing to empty dir doesn't generate an error
	dockerCmd(c, "--config", cDir, "ps")

	// Test with env var too
	cmd := exec.Command(dockerBinary, "ps")
	cmd.Env = appendBaseEnv(true, "DOCKER_CONFIG="+cDir)
	out, _, err := runCommandWithOutput(cmd)

	c.Assert(err, checker.IsNil, check.Commentf("ps2 didn't work,out:%v", out))

	// Start a server so we can check to see if the config file was
	// loaded properly
	var headers map[string][]string

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			headers = r.Header
		}))
	defer server.Close()

	// Create a dummy config file in our new config dir
	data := `{
		"HttpHeaders": { "MyHeader": "MyValue" }
	}`

	tmpCfg := filepath.Join(cDir, "config.json")
	err = ioutil.WriteFile(tmpCfg, []byte(data), 0600)
	c.Assert(err, checker.IsNil, check.Commentf("Err creating file"))

	env := appendBaseEnv(false)

	cmd = exec.Command(dockerBinary, "--config", cDir, "-H="+server.URL[7:], "ps")
	cmd.Env = env
	out, _, err = runCommandWithOutput(cmd)

	c.Assert(err, checker.NotNil, check.Commentf("out:%v", out))
	c.Assert(headers["Myheader"], checker.NotNil)
	c.Assert(headers["Myheader"][0], checker.Equals, "MyValue", check.Commentf("ps3 - Missing header,out:%v", out))

	// Reset headers and try again using env var this time
	headers = map[string][]string{}
	cmd = exec.Command(dockerBinary, "-H="+server.URL[7:], "ps")
	cmd.Env = append(env, "DOCKER_CONFIG="+cDir)
	out, _, err = runCommandWithOutput(cmd)

	c.Assert(err, checker.NotNil, check.Commentf("%v", out))
	c.Assert(headers["Myheader"], checker.NotNil)
	c.Assert(headers["Myheader"][0], checker.Equals, "MyValue", check.Commentf("ps4 - Missing header,out:%v", out))

	// Reset headers and make sure flag overrides the env var
	headers = map[string][]string{}
	cmd = exec.Command(dockerBinary, "--config", cDir, "-H="+server.URL[7:], "ps")
	cmd.Env = append(env, "DOCKER_CONFIG=MissingDir")
	out, _, err = runCommandWithOutput(cmd)

	c.Assert(err, checker.NotNil, check.Commentf("out:%v", out))
	c.Assert(headers["Myheader"], checker.NotNil)
	c.Assert(headers["Myheader"][0], checker.Equals, "MyValue", check.Commentf("ps5 - Missing header,out:%v", out))

	// Reset headers and make sure flag overrides the env var.
	// Almost same as previous but make sure the "MissingDir" isn't
	// ignore - we don't want to default back to the env var.
	headers = map[string][]string{}
	cmd = exec.Command(dockerBinary, "--config", "MissingDir", "-H="+server.URL[7:], "ps")
	cmd.Env = append(env, "DOCKER_CONFIG="+cDir)
	out, _, err = runCommandWithOutput(cmd)

	c.Assert(err, checker.NotNil, check.Commentf("out:%v", out))
	c.Assert(headers["Myheader"], checker.IsNil, check.Commentf("ps6 - Headers shouldn't be the expected value,out:%v", out))
}
