package docs

import (
	"bytes"
	"fmt"
	"reflect"
	"text/template"

	"github.com/Jeffail/benthos/v3/lib/util/config"
	"github.com/Jeffail/gabs/v2"
	"gopkg.in/yaml.v3"
)

// ComponentSpec describes a Benthos component.
type ComponentSpec struct {
	// Name of the component
	Name string

	// Type of the component (input, output, etc)
	Type string

	// Summary of the component (in markdown, must be short).
	Summary string

	// Description of the component (in markdown).
	Description string

	Fields FieldSpecs
}

type fieldContext struct {
	Name          string
	Type          string
	Description   string
	Advanced      bool
	Deprecated    bool
	Interpolation FieldInterpolation
	Examples      []string
	Options       []string
}

type componentContext struct {
	Name           string
	Type           string
	Summary        string
	Description    string
	Fields         []fieldContext
	CommonConfig   string
	AdvancedConfig string
}

func (ctx fieldContext) InterpolationBatchWide() FieldInterpolation {
	return FieldInterpolationBatchWide
}

func (ctx fieldContext) InterpolationIndividual() FieldInterpolation {
	return FieldInterpolationIndividual
}

var componentTemplate = `---
title: {{.Name}}
type: {{.Type}}
---

<!--
     THIS FILE IS AUTOGENERATED!

     To make changes please edit the contents of:
     lib/{{.Type}}/{{.Name}}.go
-->

{{if gt (len .Summary) 0 -}}
{{.Summary}}
{{end}}
{{if eq .CommonConfig .AdvancedConfig -}}
` + "```yaml" + `
{{.CommonConfig -}}
` + "```" + `
{{else}}
import Tabs from '@theme/Tabs';

<Tabs defaultValue="common" values={{"{"}}[
  { label: 'Common', value: 'common', },
  { label: 'Advanced', value: 'advanced', },
]{{"}"}}>

import TabItem from '@theme/TabItem';

<TabItem value="common">

` + "```yaml" + `
{{.CommonConfig -}}
` + "```" + `

</TabItem>
<TabItem value="advanced">

` + "```yaml" + `
{{.AdvancedConfig -}}
` + "```" + `

</TabItem>
</Tabs>
{{end -}}
{{if gt (len .Description) 0}}
{{.Description}}
{{end}}
{{if gt (len .Fields) 0 -}}
## Fields

{{end -}}
{{range $i, $field := .Fields -}}
### ` + "`{{$field.Name}}`" + `

` + "`{{$field.Type}}`" + ` {{$field.Description}}
{{if gt (len $field.Options) 0}}
Options are: {{range $j, $option := $field.Options -}}
{{if ne $j 0}}, {{end}}` + "`" + `{{$option}}` + "`" + `{{end}}.
{{end}}
{{if eq $field.Interpolation .InterpolationBatchWide -}}
This field supports [interpolation functions](/docs/configuration/interpolation#functions) that are resolved batch wide.

{{end -}}
{{if eq $field.Interpolation .InterpolationIndividual -}}
This field supports [interpolation functions](/docs/configuration/interpolation#functions).

{{end -}}
{{if gt (len $field.Examples) 0 -}}
` + "```yaml" + `
# Examples

{{range $j, $example := $field.Examples -}}
{{if ne $j 0}}
{{end}}{{$example}}{{end -}}
` + "```" + `

{{end -}}
{{end}}
`

func (c *ComponentSpec) createConfigs(root string, fullConfigExample interface{}) (
	advancedConfigBytes, commonConfigBytes []byte,
) {
	var err error
	if len(c.Fields) > 0 {
		advancedConfig, err := c.Fields.ConfigAdvanced(fullConfigExample)
		if err == nil {
			tmp := map[string]interface{}{
				c.Name: advancedConfig,
			}
			if len(root) > 0 {
				tmp = map[string]interface{}{
					root: tmp,
				}
			}
			advancedConfigBytes, err = config.MarshalYAML(tmp)
		}
		var commonConfig interface{}
		if err == nil {
			commonConfig, err = c.Fields.ConfigCommon(advancedConfig)
		}
		if err == nil {
			tmp := map[string]interface{}{
				c.Name: commonConfig,
			}
			if len(root) > 0 {
				tmp = map[string]interface{}{
					root: tmp,
				}
			}
			commonConfigBytes, err = config.MarshalYAML(tmp)
		}
	}
	if err != nil {
		panic(err)
	}
	if len(c.Fields) == 0 {
		tmp := map[string]interface{}{
			c.Name: fullConfigExample,
		}
		if len(root) > 0 {
			tmp = map[string]interface{}{
				root: tmp,
			}
		}
		if advancedConfigBytes, err = config.MarshalYAML(tmp); err != nil {
			panic(err)
		}
		commonConfigBytes = advancedConfigBytes
	}
	return
}

// AsMarkdown renders the spec of a component, along with a full configuration
// example, into a markdown document.
func (c *ComponentSpec) AsMarkdown(nest bool, fullConfigExample interface{}) ([]byte, error) {
	ctx := componentContext{
		Name:        c.Name,
		Type:        c.Type,
		Summary:     c.Summary,
		Description: c.Description,
	}

	if tmpBytes, err := yaml.Marshal(fullConfigExample); err == nil {
		fullConfigExample = map[string]interface{}{}
		if err = yaml.Unmarshal(tmpBytes, &fullConfigExample); err != nil {
			panic(err)
		}
	} else {
		panic(err)
	}

	root := ""
	if nest {
		root = c.Type
	}

	advancedConfigBytes, commonConfigBytes := c.createConfigs(root, fullConfigExample)
	ctx.CommonConfig = string(commonConfigBytes)
	ctx.AdvancedConfig = string(advancedConfigBytes)

	gConf := gabs.Wrap(fullConfigExample)

	if len(c.Description) > 0 && c.Description[0] == '\n' {
		ctx.Description = c.Description[1:]
	}

	flattenedFields := FieldSpecs{}
	var walkFields func(path string, gObj *gabs.Container, f FieldSpecs) []string
	walkFields = func(path string, gObj *gabs.Container, f FieldSpecs) []string {
		var missingFields []string
		expectedFields := map[string]struct{}{}
		for k := range gObj.ChildrenMap() {
			expectedFields[k] = struct{}{}
		}
		for _, v := range f {
			newV := v
			delete(expectedFields, v.Name)
			newV.Children = nil
			if len(path) > 0 {
				newV.Name = path + newV.Name
			}
			flattenedFields = append(flattenedFields, newV)
			if len(v.Children) > 0 {
				missingFields = append(missingFields, walkFields(v.Name+".", gConf.S(v.Name), v.Children)...)
			}
		}
		for k := range expectedFields {
			missingFields = append(missingFields, path+k)
		}
		return missingFields
	}
	if len(c.Fields) > 0 {
		if missing := walkFields("", gConf, c.Fields); len(missing) > 0 {
			return nil, fmt.Errorf("spec missing fields: %v", missing)
		}
	}

	for _, v := range flattenedFields {
		if v.Deprecated {
			continue
		}

		if !gConf.ExistsP(v.Name) {
			return nil, fmt.Errorf("unrecognised field '%v'", v.Name)
		}

		fieldType := v.Type
		if len(fieldType) == 0 {
			if len(v.Examples) > 0 {
				fieldType = reflect.TypeOf(v.Examples[0]).Kind().String()
			} else {
				if c := gConf.Path(v.Name).Data(); c != nil {
					fieldType = reflect.TypeOf(c).Kind().String()
				} else {
					return nil, fmt.Errorf("unable to infer type of '%v'", v.Name)
				}
			}
		}
		switch fieldType {
		case "map":
			fieldType = "object"
		case "slice":
			fieldType = "array"
		case "float64", "int", "int64":
			fieldType = "number"
		}

		var examples []string
		for _, example := range v.Examples {
			exampleBytes, err := config.MarshalYAML(map[string]interface{}{
				v.Name: example,
			})
			if err != nil {
				return nil, err
			}
			examples = append(examples, string(exampleBytes))
		}

		fieldCtx := fieldContext{
			Name:          v.Name,
			Type:          fieldType,
			Description:   v.Description,
			Advanced:      v.Advanced,
			Examples:      examples,
			Options:       v.Options,
			Interpolation: v.Interpolation,
		}

		if len(fieldCtx.Description) == 0 {
			fieldCtx.Description = "Sorry! This field is missing documentation."
		}

		if fieldCtx.Description[0] == '\n' {
			fieldCtx.Description = fieldCtx.Description[1:]
		}

		ctx.Fields = append(ctx.Fields, fieldCtx)
	}

	var buf bytes.Buffer
	err := template.Must(template.New("component").Parse(componentTemplate)).Execute(&buf, ctx)

	return buf.Bytes(), err
}
