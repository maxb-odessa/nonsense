package utils

import (
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/danwakefield/fnmatch"

	"github.com/maxb-odessa/slog"
)

// load no more than "maxNum"  files with extension "ext" of max size "maxSize" from dir "dir"
// and put them into map "holder" using filename without extension
func LoadDir(holder map[string][]byte, dir string, ext string, maxSize int64, maxNum int) error {

	absDir, _ := filepath.Abs(os.ExpandEnv(dir))

	d, err := os.Open(absDir)
	if err != nil {
		return err
	}

	defer d.Close()

	files, err := d.Readdir(0)
	if err != nil {
		return err
	}

	for _, f := range files {

		if len(holder) >= maxNum {
			slog.Warn("Not loading '%s': too much files loaded", f.Name())
			break
		}

		if !fnmatch.Match(`*`+ext, f.Name(), fnmatch.FNM_PATHNAME) {
			continue
		}

		if !f.Mode().IsRegular() {
			continue
		}

		if f.Size() > maxSize {
			slog.Warn("Not loading '%s': too large", f)
			continue
		}

		path := absDir + `/` + f.Name()
		if data, err := os.ReadFile(path); err != nil {
			slog.Warn("Failed to read file '%s': %s", path, err)
			return err
		} else {
			slog.Debug(1, "Loaded file '%s'", path)
			noExt := strings.TrimSuffix(f.Name(), ext)
			holder[noExt] = data
		}

	}

	return nil
}

func HTMLSafe(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), " ", "&nbsp;")
}
