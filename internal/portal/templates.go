package portal

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

// tmpl holds every named template ("login", "dashboard", "styles") parsed
// from templates/*.html - see the {{define "name"}} at the top of each
// file. html/template (not text/template) is what makes the recap-prompt
// textarea and every other user-supplied value safe to echo back into the
// page: it escapes by context automatically, so admin-supplied text can't
// break out of an attribute or inject a script tag.
var tmpl = template.Must(template.ParseFS(templateFS, "templates/*.html"))
