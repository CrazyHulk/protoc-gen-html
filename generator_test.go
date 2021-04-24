package main

import (
	"github.com/pseudomuto/protokit"
	"html/template"
	"os"
	"testing"
)

var str = `
<h1 id={{.Name}}>{{.Name}}</h1>
<p>{{.Comments.Leading}}</p>
<ul>
	{{range .Apis}}
		<li><a href={{.Path}}>{{.Path}}</a></li>
	{{end}}
</ul>
`

type htmlObject struct {
	Name string
	Comments *protokit.Comment
	Apis []*api
}

func TestA(t *testing.T) {

	ht, err := template.New("doc").Parse(str)

	f, err := os.Create("./test.html")
	if err != nil {
		return
	}

	//ht.ExecuteTemplate(t.output, "doc", t)
	err = ht.Execute(f, htmlObject{
		Name:"fff",
		Comments:&protokit.Comment{
			Leading: "ffff",
		},
		Apis: []*api{&api{Path:"ffff"},
		}})
	//err = ht.ExecuteTemplate(f, "", twirp{
	//	Name:"fff",
	//	Comments:&protokit.Comment{
	//		Leading: "ffff",
	//	},
	//	Apis: []*api{&api{Path:"ffff"},
	//	}})
	//f.Sync()
	t.Error(err)
}