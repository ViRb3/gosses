package main

import (
	"archive/zip"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/facebookgo/symwalk"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/ziflex/lecho/v2"
	"html/template"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var host = flag.String("h", "127.0.0.1", "Host to listen to, empty for all")
var port = flag.Int("p", 8001, "Port to listen to")
var prefixPath = flag.String("prefix", "/", "Url prefix at which gosses can be reached")
var symlinks = flag.Bool("symlinks", false, "Follow symlinks. "+
	"\033[4mWARNING\033[0m: symlinks will by nature allow escaping the shared path")
var skipHidden = flag.Bool("k", true, "Skip files prefixed with '.'")
var readOnly = flag.Bool("ro", false, "Read-only mode. Disable upload, rename, move, etc")
var logJson = flag.Bool("json", false, "Output logs in JSON")

var rootPath string
var pageTemplate *template.Template

//go:embed gosses-ui/ui.tmpl
var pageHtml string

//go:embed gosses-ui/script.js
var scriptJs string

//go:embed gosses-ui/style.css
var styleCss string

//go:embed gosses-ui/favicon.svg
var faviconSvg []byte

type pageRowData struct {
	Name string
	Href string
	Size string
	Ext  string
}

type pageData struct {
	Title       string
	ExtraPath   string
	Ro          bool
	RowsFiles   []pageRowData
	RowsFolders []pageRowData
}

type rpcCall struct {
	Call string   `json:"call"`
	Args []string `json:"args"`
}

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	pageHtml = strings.Replace(pageHtml, "css_will_be_here", styleCss, 1)
	pageHtml = strings.Replace(pageHtml, "js_will_be_here", scriptJs, 1)
	pageHtml = strings.Replace(pageHtml, "favicon_will_be_here", base64.StdEncoding.EncodeToString(faviconSvg), 2)
	var err error
	pageTemplate, err = template.New("").Parse(pageHtml)
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	// ensure that prefix has single trailing slash, required by frontend
	*prefixPath = strings.TrimSuffix(*prefixPath, "/") + "/"
}

func main() {
	flag.Usage = func() {
		fmt.Printf("Usage: gosses [OPTION]... PATH_TO_SHARE\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	} else {
		var err error
		// required to ensure os.Stat filename won't be absolute (e.g. '..')
		rootPath, err = filepath.Abs(flag.Args()[0])
		if err != nil {
			panic(err)
		}
	}
	if *logJson {
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}
	serve(true)
}

func serve(block bool) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	logger := lecho.From(log.Logger)
	e.Logger = logger
	e.Use(lecho.Middleware(lecho.Config{Logger: logger}))
	e.HTTPErrorHandler = func(err error, context echo.Context) {
		context.String(500, "error")
	}

	// handleUnknown has to be defined before handleContent so if prefixPath is '/' handleContent can take precedence.
	e.GET("*", handleUnknown)

	group := e.Group(*prefixPath)
	group.POST("rpc", handleRPC, readOnlyChecker)
	group.POST("post", handleUpload, readOnlyChecker)
	group.GET("zip", handleZip)
	group.GET("*", handleContent)

	listener := func() {
		if err := e.Start(fmt.Sprintf("%s:%d", *host, *port)); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Send()
		}
	}
	log.Info().Str("state", "started http server").Send()
	if block {
		listener()
	} else {
		go listener()
	}
	return e
}

func handleUnknown(c echo.Context) error {
	return c.Redirect(302, *prefixPath)
}

func readOnlyChecker(handlerFunc echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if *readOnly {
			return c.String(403, "error")
		} else {
			return handlerFunc(c)
		}
	}
}

// Shortens byte count to human-readable alternative such as kilobytes or megabytes.
func humanize(bytes int64) string {
	b := float64(bytes)
	u := 0
	for {
		if b < 1024 {
			return strconv.FormatFloat(b, 'f', 1, 64) + [9]string{"B", "k", "M", "G", "T", "P", "E", "Z", "Y"}[u]
		}
		b = b / 1024
		u++
	}
}

// Handles content requests from the frontend.
// If the file is a directory, it will be listed, otherwise it will be served directly.
func handleContent(c echo.Context) error {
	filePath := resolvePath(c.Request().URL.Path)
	stat, err := osStat(filePath)
	if os.IsNotExist(err) {
		return c.String(404, "error")
	} else if err != nil {
		return err
	}
	// error on hidden files but not current directory '.'
	if *skipHidden && strings.HasPrefix(stat.Name(), ".") {
		return c.String(404, "error")
	}
	if !stat.IsDir() {
		http.ServeFile(c.Response().Writer, c.Request(), filePath)
	} else {
		if err := handleListDir(c, filePath); err != nil {
			return err
		}
	}
	return nil
}

// Handles a directory list from the frontend.
func handleListDir(c echo.Context, filePath string) error {
	p := pageData{
		// leading slash is required by frontend
		Title:     "/",
		ExtraPath: *prefixPath,
		Ro:        *readOnly,
	}
	rel, err := filepath.Rel(rootPath, filePath)
	if err != nil {
		return err
	}
	if rel != "." {
		p.RowsFolders = append(p.RowsFolders, pageRowData{"../", "../", "", "folder"})
		// trailing slash is required by frontend
		p.Title += filepath.ToSlash(rel + "/")
	}
	files, err := os.ReadDir(filePath)
	if err != nil {
		return err
	}
	for _, file := range files {
		if *skipHidden && strings.HasPrefix(file.Name(), ".") {
			continue
		}
		fileStat, err := osStat(filepath.Join(filePath, file.Name()))
		if err != nil {
			return err
		}
		if fileStat.IsDir() {
			p.RowsFolders = append(p.RowsFolders, pageRowData{
				// trailing slash is required by frontend
				file.Name() + "/",
				file.Name(),
				"",
				"folder",
			})
		} else {
			p.RowsFiles = append(p.RowsFiles, pageRowData{
				file.Name(),
				file.Name(),
				humanize(fileStat.Size()),
				strings.TrimLeft(filepath.Ext(file.Name()), "."),
			})
		}
	}
	if err := pageTemplate.Execute(c.Response().Writer, &p); err != nil {
		return err
	}
	c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTMLCharsetUTF8)
	return nil
}

// Handles a directory ZIP download from the frontend.
// The archive will be created with no compression (Store) to avoid any performance impact.
func handleZip(c echo.Context) error {
	zipPath := c.QueryParam("zipPath")
	zipName := c.QueryParam("zipName")
	zipFullPath := resolvePath(zipPath)
	if _, err := osStat(zipFullPath); os.IsNotExist(err) {
		return c.String(404, "error")
	} else if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Disposition", "attachment; filename=\""+zipName+".zip\"")
	zipWriter := zip.NewWriter(c.Response().Writer)
	defer zipWriter.Close()
	if err := osWalk(zipFullPath, func(path string, f fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if *skipHidden && strings.HasPrefix(f.Name(), ".") {
			if f.IsDir() {
				return filepath.SkipDir
			} else {
				return nil
			}
		}
		header, err := zip.FileInfoHeader(f)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Join(zipFullPath, ".."), path)
		if err != nil {
			return err
		}
		// make the paths consistent between OSes
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Store
		headerWriter, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		if f.IsDir() {
			// no data needs to be written to directory
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		if _, err := io.Copy(headerWriter, file); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// Handles a file upload from the frontend.
func handleUpload(c echo.Context) error {
	unescapedPath, err := url.PathUnescape(c.Request().Header.Get("gossa-path"))
	if err != nil {
		return err
	}
	dstPath := resolvePath(unescapedPath)
	reader, err := c.Request().MultipartReader()
	if err != nil {
		return err
	}
	srcFile, err := reader.NextPart()
	if err != nil {
		return err
	}
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return c.String(200, "ok")
}

// Handles an RPC call from the frontend.
func handleRPC(c echo.Context) error {
	bodyBytes, err := ioutil.ReadAll(c.Request().Body)
	if err != nil {
		return err
	}
	var rpc rpcCall
	if err := json.Unmarshal(bodyBytes, &rpc); err != nil {
		return err
	}
	switch rpc.Call {
	case "mkdirp":
		err = os.MkdirAll(resolvePath(rpc.Args[0]), os.ModePerm)
	case "mv":
		err = os.Rename(resolvePath(rpc.Args[0]), resolvePath(rpc.Args[1]))
	case "rm":
		err = os.RemoveAll(resolvePath(rpc.Args[0]))
	default:
		return errors.New("unknown rpc call")
	}
	if err != nil {
		return err
	}
	return c.String(200, "ok")
}

func osStat(name string) (os.FileInfo, error) {
	if *symlinks {
		return os.Stat(name)
	} else {
		return os.Lstat(name)
	}
}

func osWalk(path string, walkFn filepath.WalkFunc) error {
	if *symlinks {
		return symwalk.Walk(path, walkFn)
	} else {
		return filepath.Walk(path, walkFn)
	}
	return nil
}

// Resolves file paths relative to the rootPath, stripping away the prefixPath.
// Accounts for symlinks, if enabled.
// Prevents any directory traversal attacks.
func resolvePath(unsafePath string) string {
	unsafePath, err := filepath.Rel(*prefixPath, filepath.Clean("//"+unsafePath))
	if err != nil {
		panic(err)
	}
	newPath := filepath.Join(rootPath, filepath.Clean("//"+unsafePath))
	if *symlinks {
		evalNewPath, err := filepath.EvalSymlinks(newPath)
		if err == nil && evalNewPath != "" {
			newPath = evalNewPath
		}
	}
	return newPath
}
