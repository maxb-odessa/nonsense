package tmpl

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	"github.com/maxb-odessa/nonsense/internal/utils"

	"github.com/maxb-odessa/slog"
)

type Tmpl *template.Template
type Tmpls map[string]Tmpl

func Load(dir string) (Tmpls, error) {

	files := make(map[string][]byte)

	// precaution: load no more than 32 files max 64k bytes each
	if err := utils.LoadDir(files, dir, ".tmpl", 65536, 32); err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, errors.New("No HTML templates loaded")
	}

	templates := make(Tmpls)

	for n, t := range files {
		if tmpl, err := template.New(n).Parse(string(t)); err != nil {
			return nil, err
		} else {
			templates[n] = tmpl
			slog.Debug(9, "added temlpate '%s'", n)
		}
	}

	return templates, nil
}

func Apply(tm Tmpl, data interface{}) (string, error) {
	result := ""

	var buf bytes.Buffer
	t := template.Template(*tm)
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	} else {
		result = buf.String()
		//result = strings.ReplaceAll(buf.String(), "\n", "")
		// result = template.HTML(res)
		slog.Debug(9, "string after templating: '%s'", result)
	}

	return result, nil
}

func ApplyByName(target string, tms Tmpls, data interface{}) (string, error) {
	if tm, ok := tms[target]; ok {
		return Apply(tm, data)
	}
	return "", fmt.Errorf("Template '%s' is not loaded", target)
}
