//go:generate statik -src=./public

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/apex/log"
	jlog "github.com/apex/log/handlers/json"
	"github.com/apex/log/handlers/text"
	"github.com/gorilla/pat"
	_ "github.com/kaihendry/lk2/statik"
	"github.com/nfnt/resize"
	"github.com/pyk/byten"
	"github.com/rakyll/statik/fs"
	"github.com/skratchdot/open-golang/open"
)

var dirPath = "."
var dirThumbs = fmt.Sprintf("%s%s", os.Getenv("HOME"), "/.cache/lk")
var dirTrash = fmt.Sprintf("%s%s", os.Getenv("HOME"), "/.Trash")
var port = flag.Int("port", 0, "listen port")
var openbrowser = flag.Bool("openbrowser", true, "Open in browser")

type media struct {
	Filename string `json:"filename"`
	fileinfo os.FileInfo
	Ext      string `json:"ext"`
	Size     string `json:"size"`
}

func hostname() string {
	hostname, _ := os.Hostname()
	// If hostname does not have dots (i.e. not fully qualified), then return zeroconf address for LAN browsing
	if strings.Split(hostname, ".")[0] == hostname {
		return hostname + ".local"
	}
	return hostname
}

func init() {
	if os.Getenv("UP_STAGE") == "" {
		log.SetHandler(text.Default)
	} else {
		log.SetHandler(jlog.Default)
	}
}

func main() {
	flag.Parse()

	statikFS, err := fs.New()
	if err != nil {
		log.WithError(err).Fatal("error compiling static resources")
	}

	app := pat.New()

	app.Get("/get", get)
	app.Get("/t/", thumb)
	app.Post("/trash", trash)
	app.Delete("/", delete)

	directory := flag.Arg(0)
	dirPath, _ = filepath.Abs(directory)

	// Getting rid of /../ etc
	dirPath = path.Clean(dirPath)

	// Don't allow path under dirPath to be viewed
	app.PathPrefix("/o/").Handler(
		http.StripPrefix(path.Join("/o", dirPath), http.FileServer(http.Dir(dirPath))))

	app.PathPrefix("/").Handler(
		http.StripPrefix("/", http.FileServer(statikFS)))

	// http://stackoverflow.com/a/33985208/4534
	eport := os.Getenv("PORT")
	if eport != "" {
		*port, _ = strconv.Atoi(eport)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.WithError(err).Fatal("Failed to listen")
	}

	if a, ok := ln.Addr().(*net.TCPAddr); ok {
		host := fmt.Sprintf("http://%s:%d", hostname(), a.Port)
		fmt.Println("Serving from", host)
		if *openbrowser {
			open.Start(host)
		}

	}
	if err := http.Serve(ln, app); err != nil {
		log.WithError(err).Fatal("Failed to serve")
	}
}

func get(w http.ResponseWriter, r *http.Request) {

	var m []media
	err := filepath.Walk(dirPath, findmedia(&m))

	// Largest file first
	sort.Slice(m, func(i, j int) bool {
		return m[i].fileinfo.Size() > m[j].fileinfo.Size()
	})

	log.WithFields(log.Fields{
		"items": len(m),
	}).Info("get media")

	if err != nil {
		log.WithError(err).Fatal("error walking")
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(m)
	return

}

func findmedia(m *[]media) func(filename string, f os.FileInfo, err error) error {
	return func(filename string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		base := filepath.Base(filename)
		if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") {
			// Skip hidden files
			return nil
		}
		// log.Printf("Visited: %s\n", filename)
		if f.IsDir() {
			return nil
		}

		ext := strings.ToLower(path.Ext(filename))

		switch ext {
		case ".jpg":
			*m = append(*m, media{filename, f, ext, byten.Size(f.Size())})
		case ".png":
			*m = append(*m, media{filename, f, ext, byten.Size(f.Size())})
		case ".mp4":
			*m = append(*m, media{filename, f, ext, byten.Size(f.Size())})
		default:
			// fmt.Printf("ignoring %s, with ext %s.", filename, ext)
		}
		return nil
	}
}
func thumb(w http.ResponseWriter, r *http.Request) {

	// Path cleaning
	requestedPath := path.Clean(r.URL.Path[2:])

	// Make sure you can't go under the dirPath
	if !strings.HasPrefix(requestedPath, dirPath) {
		http.NotFound(w, r)
		return
	}

	thumbPath := filepath.Join(dirThumbs, requestedPath)
	if _, err := os.Stat(thumbPath); err != nil {
		log.WithError(err).Warnf("THUMB: %s does not exist", thumbPath)
		srcPath := requestedPath
		if _, err := os.Stat(srcPath); err != nil {
			log.WithError(err).Warnf("original: %s does not exist", srcPath)
			http.NotFound(w, r)
			return
		}

		log.Infof("Must generate thumb for %s", srcPath)
		err := genthumb(srcPath, thumbPath)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		log.Infof("Created thumb %s", thumbPath)
	}
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, thumbPath)
}

func genthumb(src string, dst string) (err error) {

	dir, _ := filepath.Split(dst)
	err = os.MkdirAll(dir, 0700)
	if err != nil {
		return err
	}

	switch mediatype := strings.ToLower(path.Ext(src)); mediatype {
	case ".jpg":
		return genJPGthumb(src, dst)
	case ".mp4":
		path, err := exec.LookPath("ffmpeg")
		if err != nil {
			path, err = exec.LookPath("./ffmpeg")
		}
		if err == nil {
			out, err := exec.Command(path, "-y", "-ss", "0.5", "-i", src, "-vframes", "1", "-f", "image2", dst).CombinedOutput()
			if err != nil {
				log.WithError(err).Warnf("ffmpeg failed: %s", out)
			}
		}
		return err
	default:
		return fmt.Errorf("unknown mediatype: %s", mediatype)
	}
}

func genJPGthumb(src string, dst string) (err error) {

	// First if vipsthumbnail is around, use that, because it's crazy fast
	path, err := exec.LookPath("vipsthumbnail")
	if err == nil {
		out, err := exec.Command(path, "-t", "-s", "460x460", "-o", dst, src).CombinedOutput()
		if err != nil {
			fmt.Printf("Command output is %s\n", out)
			return err
		}
		return err
	}

	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	img, err := jpeg.Decode(file)
	if err != nil {
		return err
	}

	m := resize.Thumbnail(460, 460, img, resize.NearestNeighbor)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	// write new image to file
	jpeg.Encode(out, m, nil)

	return
}

func delete(w http.ResponseWriter, r *http.Request) {

	decoder := json.NewDecoder(r.Body)
	var m []media
	err := decoder.Decode(&m)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	defer r.Body.Close()

	log.WithFields(log.Fields{
		"m": m,
	}).Info("delete")

	for _, v := range m {

		err = os.Remove(v.Filename)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(m)
	return

}

func trash(w http.ResponseWriter, r *http.Request) {

	decoder := json.NewDecoder(r.Body)
	var m []media
	err := decoder.Decode(&m)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	defer r.Body.Close()

	log.WithFields(log.Fields{
		"m": m,
	}).Info("trash")

	for _, v := range m {
		trashPath := filepath.Join(dirTrash, v.Filename)
		log.WithFields(log.Fields{
			"src":  v.Filename,
			"dest": trashPath,
		}).Info("trash")

		err = movefile(v.Filename, trashPath)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}

	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(m)
	return

}

func movefile(source string, dest string) error {
	os.MkdirAll(path.Dir(dest), 0700)

	// First try renaming the file
	if err := os.Rename(source, dest); err == nil {
		return nil
	}

	// Try copy instead
	fr, err := os.Open(source)

	if err != nil {
		return err
	}

	defer fr.Close()

	fw, err := os.Create(dest)

	if err != nil {
		return err
	}

	_, err = io.Copy(fw, fr)

	fw.Close()

	// Remove source after copy
	fr.Close()

	if err != nil {
		return err
	}

	return os.Remove(source)
}
