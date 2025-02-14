package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func dieMaybe(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func trimSpaces(str string) string {
	space := regexp.MustCompile(`\s+`)
	return space.ReplaceAllString(str, " ")
}

func getRaw(t *testing.T, url string) []byte {
	resp, err := http.Get(url)
	dieMaybe(t, err)
	body, err := ioutil.ReadAll(resp.Body)
	dieMaybe(t, err)
	return body
}

func get(t *testing.T, url string) string {
	body := getRaw(t, url)
	return trimSpaces(string(body))
}

func postDummyFile(t *testing.T, url string, path string, payload string) string {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fileWriter, err := w.CreateFormFile("file", "file")
	dieMaybe(t, err)
	_, err = fileWriter.Write([]byte(payload))
	dieMaybe(t, err)
	err = w.Close()
	dieMaybe(t, err)
	req, err := http.NewRequest("POST", url+"post", &b)
	dieMaybe(t, err)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Gossa-Path", path)

	resp, err := http.DefaultClient.Do(req)
	dieMaybe(t, err)
	defer resp.Body.Close()
	bodyS, err := ioutil.ReadAll(resp.Body)
	dieMaybe(t, err)
	return trimSpaces(string(bodyS))
}

func postJSON(t *testing.T, url string, what string) string {
	resp, err := http.Post(url, "application/json", bytes.NewBuffer([]byte(what)))
	dieMaybe(t, err)
	body, err := ioutil.ReadAll(resp.Body)
	dieMaybe(t, err)
	return trimSpaces(string(body))
}

func fetchAndTestDefault(t *testing.T, url string) string {
	body0 := get(t, url)

	if !strings.Contains(body0, `<title>/</title>`) {
		t.Fatal("error title")
	}

	if !strings.Contains(body0, `<h1 onclick="return titleClick(event)">./</h1>`) {
		t.Fatal("error header")
	}

	if !strings.Contains(body0, `href="hols">hols/</a>`) {
		t.Fatal("error hols folder")
	}

	if !strings.Contains(body0, `href="curimit@gmail.com%20%2840%25%29">curimit@gmail.com (40%)/</a>`) {
		t.Fatal("error curimit@gmail.com (40%) folder")
	}

	if !strings.Contains(body0, `href="%e4%b8%ad%e6%96%87">中文/</a>`) {
		t.Fatal("error 中文 folder")
	}

	if !strings.Contains(body0, `href="custom_mime_type.types">custom_mime_type.types</a>`) {
		t.Fatal("error row custom_mime_type")
	}

	return body0
}

func doTestRegular(t *testing.T, url string, testExtra bool) {
	var payload, path, body0, body1, body2 string

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test fetching default path")
	fetchAndTestDefault(t, url)

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test fetching an invalid path - redirected to root")
	fetchAndTestDefault(t, url+"../../")
	fetchAndTestDefault(t, url+"hols/../../")

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test fetching regular files")
	body0 = get(t, url+"subdir_with%20space/file_with%20space.html")
	body1 = get(t, url+"fancy-path/a")
	if body0 != `<b>spacious!!</b> ` || body1 != `fancy! ` {
		t.Fatal("fetching a regular file errored")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test fetching a invalid file")
	path = "../../../../../../../../../../etc/passwd"
	if get(t, url+path) != `error` {
		t.Fatal("fetching a invalid file didnt errored")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test zip")
	bodyRaw := getRaw(t, url+"zip?zipPath=%2F%E4%B8%AD%E6%96%87%2F&zipName=%E4%B8%AD%E6%96%87")
	// can't safely use hash due to subtle changes across CI environments such as mod time and attributes
	if len(bodyRaw) != 304 {
		t.Fatal("invalid zip length", len(bodyRaw))
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test zip invalid path")
	body0 = get(t, url+"zip?zipPath=%2Ftmp&zipName=subdir")
	if body0 != `error` {
		t.Fatal("zip passed for invalid path")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test mkdir rpc")
	body0 = postJSON(t, url+"rpc", `{"call":"mkdirp","args":["/AAA"]}`)
	if body0 != `ok` {
		t.Fatal("mkdir rpc errored")
	}

	body0 = fetchAndTestDefault(t, url)
	if !strings.Contains(body0, `href="AAA">AAA/</a>`) {
		t.Fatal("mkdir rpc folder not created")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test invalid mkdir rpc")
	body0 = postJSON(t, url+"rpc", `{"call":"mkdirp","args":["../BBB"]}`)
	if body0 != `ok` {
		t.Fatal("invalid mkdir rpc errored #0")
	}

	body0 = fetchAndTestDefault(t, url)
	if !strings.Contains(body0, `href="BBB">BBB/</a>`) {
		t.Fatal("invalid mkdir rpc folder not created #0")
	}

	body0 = postJSON(t, url+"rpc", `{"call":"mkdirp","args":["/../CCC"]}`)
	if body0 != `ok` {
		t.Fatal("invalid mkdir rpc errored #1")
	}

	body0 = fetchAndTestDefault(t, url)
	if !strings.Contains(body0, `href="CCC">CCC/</a>`) {
		t.Fatal("invalid mkdir rpc folder not created #1")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test post file")
	path = "%2F%E1%84%92%E1%85%A1%20%E1%84%92%E1%85%A1" // "하 하" encoded
	payload = "123 하"
	body0 = postDummyFile(t, url, path, payload)
	body1 = get(t, url+path)
	body2 = fetchAndTestDefault(t, url)
	if body0 != `ok` || body1 != payload || !strings.Contains(body2, `href="%e1%84%92%e1%85%a1%20%e1%84%92%e1%85%a1">하 하</a>`) {
		t.Fatal("post file errored")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test post file incorrect path")
	body0 = postDummyFile(t, url, "%2E%2E"+path+"2", payload)
	body1 = get(t, url+path)
	body2 = fetchAndTestDefault(t, url)
	if body0 != `ok` || body1 != payload || !strings.Contains(body2, `href="%e1%84%92%e1%85%a1%20%e1%84%92%e1%85%a12">하 하2</a>`) {
		t.Fatal("post file incorrect path errored")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test mv rpc")
	body0 = postJSON(t, url+"rpc", `{"call":"mv","args":["/AAA", "/hols/AAA"]}`)
	body1 = fetchAndTestDefault(t, url)
	if body0 != `ok` || strings.Contains(body1, `href="AAA">AAA/</a></td> </tr>`) {
		t.Fatal("mv rpc errored")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test upload in new folder")
	payload = "test"
	body0 = postDummyFile(t, url, "%2Fhols%2FAAA%2Fabcdef", payload)
	body1 = get(t, url+"hols/AAA/abcdef")
	if body0 != `ok` || body1 != payload {
		t.Fatal("upload in new folder errored")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test symlink, should succeed: ", testExtra)
	body0 = get(t, url+"/symlink-test/e.html")
	hasReadme := strings.Contains(body0, `<b>e!!</b>`)
	if !testExtra && hasReadme {
		t.Fatal("error symlink reached where illegal")
	} else if testExtra && !hasReadme {
		t.Fatal("error symlink unreachable")
	}

	if testExtra {
		fmt.Println("\r\n~~~~~~~~~~ test symlink mkdir & cleanup")
		body0 = postJSON(t, url+"rpc", `{"call":"mkdirp","args":["/symlink-test/testfolder"]}`)
		if body0 != `ok` {
			t.Fatal("error symlink mkdir")
		}

		body0 = postJSON(t, url+"rpc", `{"call":"rm","args":["/symlink-test/testfolder"]}`)
		if body0 != `ok` {
			t.Fatal("error symlink rm")
		}
	}

	fmt.Println("\r\n~~~~~~~~~~ test hidden file, should succeed: ", testExtra)
	body0 = get(t, url+"/.testhidden")
	hasHidden := strings.Contains(body0, `test`)
	if !testExtra && hasHidden {
		t.Fatal("error hidden file reached where illegal")
	} else if testExtra && !hasHidden {
		t.Fatal("error hidden file unreachable")
	}

	//
	fmt.Println("\r\n~~~~~~~~~~ test upload in new folder")
	payload = "test"
	body0 = postDummyFile(t, url, "%2Fhols%2FAAA%2Fabcdef", payload)
	body1 = get(t, url+"hols/AAA/abcdef")
	if body0 != `ok` || body1 != payload {
		t.Fatal("upload in new folder errored")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test rm rpc & cleanup")
	body0 = postJSON(t, url+"rpc", `{"call":"rm","args":["/hols/AAA"]}`)
	if body0 != `ok` {
		t.Fatal("cleanup errored #0")
	}

	body0 = get(t, url+"hols/AAA")
	if !strings.Contains(body0, `error`) {
		t.Fatal("cleanup errored #1")
	}

	body0 = postJSON(t, url+"rpc", `{"call":"rm","args":["/BBB"]}`)
	if body0 != `ok` {
		t.Fatal("cleanup errored #2")
	}

	body0 = postJSON(t, url+"rpc", `{"call":"rm","args":["/CCC"]}`)
	if body0 != `ok` {
		t.Fatal("cleanup errored #3")
	}

	body0 = postJSON(t, url+"rpc", `{"call":"rm","args":["/하 하"]}`)
	if body0 != `ok` {
		t.Fatal("cleanup errored #4")
	}

	body0 = postJSON(t, url+"rpc", `{"call":"rm","args":["/하 하2"]}`)
	if body0 != `ok` {
		t.Fatal("cleanup errored #5")
	}

	fmt.Printf("\r\n=========\r\n")
}

func doTestReadonly(t *testing.T, url string) {
	var payload, path, body0, body1 string

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test fetching default path")
	fetchAndTestDefault(t, url)

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test fetching an invalid path - redirected to root")
	fetchAndTestDefault(t, url+"../../")
	fetchAndTestDefault(t, url+"hols/../../")

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test fetching regular files")
	body0 = get(t, url+"subdir_with%20space/file_with%20space.html")
	body1 = get(t, url+"fancy-path/a")
	if body0 != `<b>spacious!!</b> ` || body1 != `fancy! ` {
		t.Fatal("fetching a regular file errored")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test fetching a invalid file")
	path = "../../../../../../../../../../etc/passwd"
	if get(t, url+path) != `error` {
		t.Fatal("fetching a invalid file didnt errored")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test mkdir rpc")
	body0 = postJSON(t, url+"rpc", `{"call":"mkdirp","args":["/AAA"]}`)
	if body0 == `ok` {
		t.Fatal("mkdir rpc passed - should not be allowed")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test post file")
	path = "%2F%E1%84%92%E1%85%A1%20%E1%84%92%E1%85%A1" // "하 하" encoded
	payload = "123 하"
	body0 = postDummyFile(t, url, path, payload)
	body1 = get(t, url+path)
	if body0 == `ok` {
		t.Fatal("post file passed - should not be allowed")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test mv rpc")
	body0 = postJSON(t, url+"rpc", `{"call":"mv","args":["/AAA", "/hols/AAA"]}`)
	body1 = fetchAndTestDefault(t, url)
	if body0 == `ok` {
		t.Fatal("mv rpc passed - should not be allowed")
	}

	// ~~~~~~~~~~~~~~~~~
	fmt.Println("\r\n~~~~~~~~~~ test rm rpc & cleanup")
	body0 = postJSON(t, url+"rpc", `{"call":"rm","args":["/hols/AAA"]}`)
	if body0 == `ok` {
		t.Fatal("cleanup passed - should not be allowed")
	}

	fmt.Printf("\r\n=========\r\n")
}

func TestMain(m *testing.M) {
	*host = "127.0.0.1"
	*port = 8001
	rootPath = "test-fixture"
	os.Exit(m.Run())
}

func TestNormal(t *testing.T) {
	*readOnly = false
	*symlinks = false
	*skipHidden = true
	*prefixPath = "/"
	fmt.Println("========== testing normal path ============")
	autoServe(t, func() {
		doTestRegular(t, "http://127.0.0.1:8001/", false)
	})
}

func TestExtra(t *testing.T) {
	*readOnly = false
	*symlinks = true
	*skipHidden = false
	*prefixPath = "/fancy-path/"
	// TODO: Symlinking will fail on Windows unless run as Administrator.
	symlinkPath := filepath.Join("test-fixture", "symlink-test")
	if err := os.Symlink(filepath.Join(".", "subdir"), symlinkPath); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(symlinkPath)
	fmt.Println("========== testing extras options ============")
	autoServe(t, func() {
		doTestRegular(t, "http://127.0.0.1:8001/fancy-path/", true)
	})
}

func TestRo(t *testing.T) {
	*readOnly = true
	*symlinks = false
	*skipHidden = true
	*prefixPath = "/"

	fmt.Println("========== testing read only ============")
	autoServe(t, func() {
		doTestReadonly(t, "http://127.0.0.1:8001/")
	})
}

func autoServe(t *testing.T, action func()) {
	e := serve(false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	for ctx.Err() == nil {
		// wait for server to start responding
		_, err := http.Get(fmt.Sprintf("http://%s:%d", *host, *port))
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatal(ctx.Err())
	} else {
		defer e.Shutdown(context.Background())
		action()
	}
}
