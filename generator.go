package main

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/ditashi/jsbeautifier-go/jsbeautifier"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
	"github.com/pseudomuto/protokit"
)

type twirp struct {
	params commandLineParams

	// Output buffer that holds the bytes we want to write out for a single file.
	// Gets reset after working on a file.
	output *bytes.Buffer

	// Map of all proto messages
	messages map[string]*message

	enums map[string]*protokit.EnumDescriptor

	// List of all APIs
	Apis []*api

	// List of all service Comments
	Comments *protokit.Comment

	// Service Name
	Name string
}

func newGenerator(params commandLineParams) *twirp {
	t := &twirp{
		params:   params,
		messages: map[string]*message{},
		enums:    map[string]*protokit.EnumDescriptor{},
		Apis:     []*api{},
		output:   bytes.NewBuffer(nil),
	}

	return t
}

func (t *twirp) Generate(in *plugin.CodeGeneratorRequest) *plugin.CodeGeneratorResponse {
	resp := new(plugin.CodeGeneratorResponse)

	t.scanAllMessages(in, resp)
	t.GenerateMarkdown(in, resp)

	return resp
}

// P forwards to g.gen.P, which prints output.
func (t *twirp) P(args ...string) {
	for _, v := range args {
		t.output.WriteString(v)
	}
	t.output.WriteByte('\n')
}

func (t *twirp) scanAllMessages(req *plugin.CodeGeneratorRequest, resp *plugin.CodeGeneratorResponse) {
	descriptors := protokit.ParseCodeGenRequest(req)

	for _, d := range descriptors {
		t.scanMessages(d)
	}
}

func (t *twirp) GenerateMarkdown(req *plugin.CodeGeneratorRequest, resp *plugin.CodeGeneratorResponse) {
	descriptors := protokit.ParseCodeGenRequest(req)

	for _, d := range descriptors {
		for _, sd := range d.GetServices() {
			t.scanService(sd)
			t.Name = *sd.Name
			for _, api := range t.Apis {
				api.Input = t.generateJsDocForMessage(api.Request)
				api.Output = t.generateJsDocForMessage(api.Reply)
			}

			t.generateDoc()

			name := strings.Replace(d.GetName(), ".proto", ".md", 1)
			resp.File = append(resp.File, &plugin.CodeGeneratorResponse_File{
				Name:    proto.String(name),
				Content: proto.String(t.output.String()),
			})
		}
	}
}

func (t *twirp) scanMessages(d *protokit.FileDescriptor) {
	for _, ed := range d.GetEnums() {
		t.scanEnum(ed)
	}

	for _, md := range d.GetMessages() {
		t.scanMessage(md)
	}
}

func (t *twirp) scanEnum(md *protokit.EnumDescriptor) {
	t.enums["."+md.GetFullName()] = md
}

func (t *twirp) scanMessage(md *protokit.Descriptor) {
	for _, smd := range md.GetMessages() {
		t.scanMessage(smd)
	}

	for _, ed := range md.GetEnums() {
		t.scanEnum(ed)
	}

	{
		fields := make([]field, len(md.GetMessageFields()))

		maps := make(map[string]*descriptor.DescriptorProto)
		for _, t := range md.NestedType {
			if t.Options.GetMapEntry() {
				pkg := md.GetPackage()
				name := fmt.Sprintf(".%s.%s.%s", pkg, md.GetName(), t.GetName())
				maps[name] = t
			}
		}

		for i, fd := range md.GetMessageFields() {
			typeName := fd.GetTypeName()
			if typeName == "" {
				typeName = fd.GetType().String()
			}

			f := field{
				Name:  fd.GetName(),
				Type:  typeName,
				Doc:   fd.GetComments().GetLeading(),
				Note:  fd.GetComments().GetTrailing(),
				Label: fd.GetLabel(),
			}

			if e, ok := t.enums[fd.GetTypeName()]; ok {
				f.Type = "TYPE_ENUM"
				parts := []string{}

				for _, v := range e.GetValues() {
					line := fmt.Sprintf("%s(=%d) %s", v.GetName(), v.GetNumber(), v.GetComments().GetTrailing())
					parts = append(parts, line)
				}

				f.Doc = strings.Join(parts, "\n")
			}

			if m, ok := maps[f.Type]; ok {
				for _, ff := range m.GetField() {
					switch ff.GetName() {
					case "key":
						f.KeyType = ff.GetType().String()
					case "value":
						typeName := ff.GetTypeName()
						if typeName == "" {
							typeName = ff.GetType().String()
						}
						f.Type = typeName
					}
				}
				f.Label = 0
			}
			fields[i] = f
		}

		t.messages[md.GetFullName()] = &message{
			Name:   md.GetName(),
			Doc:    md.GetComments().GetTrailing(),
			Fields: fields,
		}
	}
}

type message struct {
	Name   string
	Fields []field
	Label  descriptor.FieldDescriptorProto_Label
	Doc    string
}

type field struct {
	Name    string
	Type    string
	KeyType string
	Note    string
	Doc     string
	Label   descriptor.FieldDescriptorProto_Label
}

func (f field) isRepeated() bool {
	return f.Label == descriptor.FieldDescriptorProto_LABEL_REPEATED
}

type api struct {
	Method  string
	Path    string
	Doc     string
	Request *message
	Reply   *message
	Input   string
	Output  string
}

func (t api) GetInputBeautifyCode() string {
	options := jsbeautifier.DefaultOptions()
	code, _ := jsbeautifier.Beautify(&t.Input, options)
	return code
}

func (t api) GetInputBeautifyCodes() []string {
	options := jsbeautifier.DefaultOptions()
	code, _ := jsbeautifier.Beautify(&t.Input, options)
	return strings.Split(code, "\n")
}

func (t api) GetOutputBeautifyCode() string {
	options := jsbeautifier.DefaultOptions()
	code, _ := jsbeautifier.Beautify(&t.Output, options)
	return code
}

func (t api) GetOutputBeautifyCodes() []string {
	options := jsbeautifier.DefaultOptions()
	code, _ := jsbeautifier.Beautify(&t.Output, options)
	return strings.Split(code, "\n")
}

func (t *twirp) scanService(d *protokit.ServiceDescriptor) {
	t.Comments = d.Comments
	for _, md := range d.GetMethods() {
		api := api{}

		api.Method = "POST"
		api.Path = t.params.pathPrefix + "/" + d.GetFullName() + "/" + md.GetName()
		doc := md.GetComments().GetLeading()
		// 支持文档换行
		api.Doc = strings.Replace(doc, "\n", "\n\n", -1)

		inputType := md.GetInputType()[1:] // trim leading dot
		api.Request = t.messages[inputType]

		outputType := md.GetOutputType()[1:] // trim leading dot
		api.Reply = t.messages[outputType]

		t.Apis = append(t.Apis, &api)
	}
}

func getType(t string) string {
	switch t {
	case "TYPE_STRING":
		return "string"
	case "TYPE_DOUBLE", "TYPE_FLOAT":
		return "float"
	case "TYPE_BOOL":
		return "bool"
	case "TYPE_INT64", "TYPE_UINT64", "TYPE_INT32", "TYPE_UINT32":
		return "int"
	default:
		return t
	}
}

func getTypeValue(t string) string {
	switch t {
	case "TYPE_STRING":
		return ""
	case "TYPE_DOUBLE", "TYPE_FLOAT":
		return "0.0"
	case "TYPE_BOOL":
		return "false"
	case "TYPE_INT64", "TYPE_UINT64", "TYPE_INT32", "TYPE_UINT32":
		return "0"
	default:
		return ""
	}
}

func (t *twirp) generateJsDocForField(field field) string {
	var js string
	var v, vt string
	disableDoc := false

	if field.Doc != "" {
		for _, line := range strings.Split(field.Doc, "\n") {
			js += "// " + line + "\n"
		}
	}

	if field.Type == "TYPE_STRING" {
		vt = "string"
		if field.isRepeated() {
			v = `["",""]`
		} else if field.KeyType != "" {
			v = fmt.Sprintf(`{"%s":""}`, getTypeValue(field.KeyType))
			vt = fmt.Sprintf("map<%s,string>", getType(field.KeyType))
		} else {
			v = `""`
		}
	} else if field.Type == "TYPE_DOUBLE" || field.Type == "TYPE_FLOAT" {
		vt = "float"
		if field.isRepeated() {
			v = "[0.0, 0.0]"
		} else if field.KeyType != "" {
			v = fmt.Sprintf(`{"%s":0.0}`, getTypeValue(field.KeyType))
			vt = fmt.Sprintf("map<%s,float>", getType(field.KeyType))
		} else {
			v = "0.0"
		}
	} else if field.Type == "TYPE_BOOL" {
		vt = "bool"
		if field.isRepeated() {
			v = "[false, false]"
		} else if field.KeyType != "" {
			v = fmt.Sprintf(`{"%s":false}`, getTypeValue(field.KeyType))
			vt = fmt.Sprintf("map<%s,bool>", getType(field.KeyType))
		} else {
			v = "false"
		}
	} else if field.Type == "TYPE_INT64" || field.Type == "TYPE_UINT64" {
		vt = "string(int64)"
		if field.isRepeated() {
			v = `["0", "0"]`
		} else if field.KeyType != "" {
			v = fmt.Sprintf(`{"%s":"0"}`, getTypeValue(field.KeyType))
			vt = fmt.Sprintf("map<%s,string(int64)>", getType(field.KeyType))
		} else {
			v = `"0"`
		}
	} else if field.Type == "TYPE_INT32" || field.Type == "TYPE_UINT32" {
		vt = "int"
		if field.isRepeated() {
			v = "[0, 0]"
		} else if field.KeyType != "" {
			v = fmt.Sprintf(`{"%s":0}`, getTypeValue(field.KeyType))
			vt = fmt.Sprintf("map<%s,int>", getType(field.KeyType))
		} else {
			v = "0"
		}
	} else if field.Type == "TYPE_ENUM" {
		vt = "string(enum)"
		if field.isRepeated() {
			v = `["", ""]`
		} else {
			v = `""`
		}
	} else if field.Type[0] == '.' {
		m := t.messages[field.Type[1:]]
		v = t.generateJsDocForMessage(m)
		if field.isRepeated() {
			doc := fmt.Sprintf("// type:<list<%s>>", m.Name)
			if field.Note != "" {
				doc = " " + field.Note
			}
			v = "[" + doc + "\n" + v + "]"
		} else if field.KeyType != "" {
			doc := fmt.Sprintf("// type:<map<%s,%s>>", getType(field.KeyType), m.Name)
			if field.Note != "" {
				doc = " " + field.Note
			}
			v = fmt.Sprintf("{%s\n\"%s\":%s}", doc, getTypeValue(field.KeyType), v)
		}
		disableDoc = true
	} else {
		v = "UNKNOWN"
	}

	if disableDoc {
		js += fmt.Sprintf("%s: %s,", field.Name, v)
	} else {
		js += fmt.Sprintf("\"%s\": %s, // type:<%s>", field.Name, v, vt)
		if field.Note != "" {
			js = js + ", " + field.Note
		}
	}
	js = strings.Trim(js, " ")

	js += "\n"

	return js
}

func (t *twirp) generateJsDocForMessage(m *message) string {
	var js string
	js += "{\n"

	for i, field := range m.Fields {
		tf := t.generateJsDocForField(field)
		if i == len(m.Fields)-1 {
			tf = strings.Replace(tf, ", //", " //", 1)
		}
		js += tf
	}

	js += "}"

	return js
}

func (t *twirp) generateDoc() {
	t.generateHTML()
	return
	options := jsbeautifier.DefaultOptions()
	t.P("# ", t.Name)
	t.P()
	comments := strings.Split(t.Comments.Leading, "\n")
	for _, value := range comments {
		t.P(value, "  ")
	}
	t.P()
	for _, api := range t.Apis {
		anchor := strings.Replace(api.Path, "/", "", -1)
		anchor = strings.Replace(anchor, ".", "", -1)
		anchor = strings.ToLower(anchor)

		t.P(fmt.Sprintf("- [%s](#%s)", api.Path, anchor))
	}

	t.P()

	for _, api := range t.Apis {
		t.P("## ", api.Path)
		t.P()
		t.P(api.Doc)
		t.P()
		t.P("### Method")
		t.P()
		t.P(api.Method)
		t.P()
		t.P("### Request")
		t.P("```javascript")
		code, _ := jsbeautifier.Beautify(&api.Input, options)
		t.P(code)
		t.P("```")
		t.P()
		t.P("### Reply")
		t.P("```javascript")
		code, _ = jsbeautifier.Beautify(&api.Output, options)
		t.P(code)
		t.P("```")
	}
}

var temp = `
<head>
<link rel="stylesheet" href="./doc.css">
</head>

<script src="https://cdn.staticfile.org/jquery/3.5.0/jquery.js"></script>
<!-- 引入CodeMirror核心文件 -->
<script src="https://cdn.staticfile.org/codemirror/5.14.2/codemirror.min.js"></script>
<script src="https://cdn.staticfile.org/codemirror/5.14.2/mode/javascript/javascript.min.js"></script>
<script src="https://cdn.staticfile.org/codemirror/5.14.2/addon/fold/foldcode.min.js"></script>
<script src="https://cdn.staticfile.org/codemirror/5.14.2/addon/fold/foldgutter.min.js"></script>
<script src="https://cdn.staticfile.org/codemirror/5.14.2/addon/fold/brace-fold.min.js"></script>
<script src="https://cdn.staticfile.org/codemirror/5.14.2/addon/fold/indent-fold.min.js"></script>
<link rel="stylesheet" href="https://cdn.staticfile.org/codemirror/5.14.2/codemirror.min.css">
<link rel="stylesheet" href="https://cdn.staticfile.org/codemirror/5.14.2/theme/rubyblue.min.css">
<link rel="stylesheet" href="https://cdn.staticfile.org/codemirror/5.14.2/addon/fold/foldgutter.min.css">
<!-- bootstrap样式 -->
<link rel="stylesheet" href="https://cdn.staticfile.org/twitter-bootstrap/3.3.7/css/bootstrap.min.css">
<script src="https://cdn.staticfile.org/twitter-bootstrap/3.3.7/js/bootstrap.min.js"></script>
<script src="https://cdn.staticfile.org/datatables/1.10.20/js/jquery.dataTables.min.js"></script>
<link rel="stylesheet" href="./doc.css">

<h1 id='{{.Name}}'>{{.Name}}</h1>
<p>{{.Comments.Leading}}</p>
<ul>
	{{range .Apis}}
		<li><a href='#{{.Path}}'>{{.Path}}</a></li>
	{{end}}
</ul>
{{range $index, $v := .Apis}}
	<h2 id="{{.Path}}">{{.Path}}</h2>
	<h3 id="method">Method</h3>
	<p>POST</p>
	<div class="row-fluid">
	<h3 id="request" style="display:inline-block;">Request</h3><button class="sp-curl" onclick="sendReq(this)" req="reqArea{{$index}}" path={{.Path}} resp="respArea{{$index}}" aIndex={{$index}} >CURL</button>
	</div>
	<textarea id="reqArea{{$index}}" >{{.GetInputBeautifyCode}}</textarea>
	<h3 id="reply">Reply</h3>
	<textarea id="respArea{{$index}}" >{{.GetOutputBeautifyCode}}</textarea>
{{end}}

<script>
	var cmMap = {}
	{{range $i, $k := .Apis}}
		var editor = CodeMirror.fromTextArea(document.getElementById("reqArea"+ {{$i}}), {
		  theme: 'rubyblue',
		  mode: "javascript",
		  lineNumbers: true,
		  viewportMargin:Infinity,
		  foldGutter: true,
		  readOnly:false,
		  gutters: ["CodeMirror-linenumbers", "CodeMirror-foldgutter"]
		})
		editor.on('change', e => {
			document.getElementById("reqArea"+ {{$i}}).textContent = e.getValue()
		});
		var respCM = CodeMirror.fromTextArea(document.getElementById("respArea"+ {{$i}}), {
		  theme: 'rubyblue',
		  mode: "javascript",
		  lineNumbers: true,
		  viewportMargin:Infinity,
		  foldGutter: true,
		  autoRefresh: true,
		  readOnly:false,
		  gutters: ["CodeMirror-linenumbers", "CodeMirror-foldgutter"]
		});
		cmMap[{{$i}}] = respCM
	{{end}}


	var data;
	window.onload = function init() {
  		console.log("init")
		data = {{.}}
	}

	function sendReq(obj) {
		reqArea = document.getElementById(obj.getAttribute("req"));
		reqBody = JSON.parse(reqArea.textContent.split("\n").map(x => x.replace(/\/\/.*/g, "")).join(""))
	 	index = obj.getAttribute("aIndex")
		resp = $.ajax({
			type: "POST", 
			url:"http://127.0.0.1:8080/twirp"+obj.getAttribute("path"),
			data: JSON.stringify(reqBody),
			contentType: 'application/json',
			dataType: "json",
			success: function (data, status) {
				console.log(data, status)
				respArea = document.getElementById(obj.getAttribute("resp"));
				// respArea.textContent = resp.responseText 
				// $('.CodeMirror').each(function(i, el){
		 		// 	el.CodeMirror.refresh();
				// });

				console.log(index)
				console.log(JSON.stringify(data, undefined, 4))
				console.log(cmMap)
				cmMap[index].setValue(JSON.stringify(data, undefined, 4))
				cmMap[index].refresh
			}
		})

	}
</script>
`

func (t *twirp) generateHTML() {
	ht, err := template.New("doc").Parse(temp)
	if err != nil {
		return
	}
	//ht.Execute(t.output, t)
	err = ht.ExecuteTemplate(t.output, "doc", t)
	if err != nil {
		panic(err)
	}
}
