package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"regexp"
	"strings"

	template "github.com/openshift/api/template/v1"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

const (
	tplPathDefault   = "."
	tplPathUsage     = "Path to an OpenShift Template, relative or absolute"
	chartPathDefault = "."
	chartPathUsage   = "Destination directory of the Chart."
)

var (
	tplPath   string
	chartPath string

	convertCmd = &cobra.Command{
		Use:   "convert",
		Short: "Given the path to an OpenShift template file, spit out a Helm chart.",
		Long:  `Long version...`,
		RunE: func(cmd *cobra.Command, args []string) error {

			var myTemplate template.Template

			yamlFile, err := ioutil.ReadFile(filepath.Clean(tplPath))
			if err != nil {
				return fmt.Errorf("Couldn't load template: %v", err)
			}

			// Convert to json first
			jsonB, err := yaml.YAMLToJSON(yamlFile)
			checkErr(err, fmt.Sprintf("Error transforming yaml to json: \n%s", string(yamlFile)))

			err = json.Unmarshal(jsonB, &myTemplate)
			checkErr(err, "Unable to marshal template")

			// Convert myTemplate.Objects into individual files
			var templates []*chart.File
			err = objectToTemplate(&myTemplate.Objects, &myTemplate.ObjectLabels, &templates)
			checkErr(err, "Failed object to template conversion")

			// Convert myTemplate.Parameters into a yaml string map
			values := make(map[string]interface{})
			err = paramsToValues(&myTemplate.Parameters, &values, &templates)
			checkErr(err, "Failed parameter to value conversion")

			valuesAsByte, err := yaml.Marshal(values)
			checkErr(err, "Failed converting values to YAML")

			myChart := chart.Chart{
				Metadata: &chart.Metadata{
					Name:        myTemplate.ObjectMeta.Name,
					Version:     "v0.0.1",
					Description: myTemplate.ObjectMeta.Annotations["description"],
					Tags:        myTemplate.ObjectMeta.Annotations["tags"],
				},
				Templates: templates,
				Values:    values,
				Raw:       []*chart.File{{Name: "values.yaml", Data: []byte(valuesAsByte)}},
			}

			if myChart.Metadata.Name == "" {
				ext := filepath.Ext(tplPath)
				name := filepath.Base(string(tplPath))[0 : len(filepath.Base(string(tplPath)))-len(ext)]
				myChart.Metadata.Name = name
			}

			err = chartutil.SaveDir(&myChart, chartPath)
			checkErr(err, fmt.Sprintf("Failed to save chart %s", myChart.Metadata.Name))

			return nil
		},
	}
)

func init() {
	convertCmd.Flags().StringVarP(&tplPath, "template", "t", tplPathDefault, tplPathUsage)
	convertCmd.Flags().StringVarP(&chartPath, "chart", "c", chartPathDefault, chartPathUsage)
	rootCmd.AddCommand(convertCmd)
}

func checkErr(err error, msg string) {
	if err != nil {
		log.Fatalf(msg + err.Error())
	}
	return
}

// Convert the object list in the openshift template to a set of template files in the chart
func objectToTemplate(objects *[]runtime.RawExtension, templateLabels *map[string]string, templates *[]*chart.File) error {
	o := *objects

	m := make(map[string][]byte)
	separator := []byte{'-', '-', '-', '\n'}

	for _, v := range o {
		var k8sR unstructured.Unstructured
		err := json.Unmarshal([]byte(v.Raw), &k8sR)
		if err != nil {
			return fmt.Errorf(fmt.Sprintf("Failed to unmarshal Raw resource\n%v\n", v.Raw) + err.Error())
		}

		objectKind := k8sR.GetKind()
		switch objectKind {
		// ::: DeploymenyConfig Vs Deployment :::
		case "DeploymentConfig":
			log.Printf("Converting the object from: %s into 'Deployment'", k8sR.GetKind())
			// ::: Change the apiVersion
			log.Printf("Change the current apiVersion: %s ", k8sR.GetAPIVersion())
			k8sR.SetAPIVersion("apps/v1")

			// ::: Change the object kind
			log.Printf("Change the current object type: %s ", k8sR.GetKind())
			k8sR.SetKind("Deployment")

			// ::: Delete the following entries:
			//
			// 		strategy:
			// 			activeDeadlineSeconds: 1800
			// 			type: "rolling"
			//		selector:
			//		test:
			//		triggers:
			//
			// 	and might set the full path specifying all the fields: "spec","strategy" and so on
			log.Printf("Remove the 'strategy' branch from the object: %s ", k8sR.GetKind())
			myInterface, _, err := unstructured.NestedFieldNoCopy(k8sR.Object, "spec")
			if err != nil {
				return fmt.Errorf(fmt.Sprintf("\nFailed to parse the object %s with the following Error: ", k8sR.GetKind()) + err.Error())
			}
			unstructured.RemoveNestedField(myInterface.(map[string]interface{}), "strategy")
			unstructured.RemoveNestedField(myInterface.(map[string]interface{}), "test")
			unstructured.RemoveNestedField(myInterface.(map[string]interface{}), "triggers")

			//
			// Get the original selector items tree
			//
			existingSelectorMatchLabels, isSelectorExist, err := unstructured.NestedMap(myInterface.(map[string]interface{}), "selector", "matchLabels")
			if err != nil {
				checkErr(err, "failed to get the 'selector.matchLabels' from DeploymentConfig object")
			} else if isSelectorExist { // if already exist jump to the next case
				log.Printf("Skipping the Selector because is appears as already configured = %s", existingSelectorMatchLabels)
				break
			}

			existingSelectorInterface, isSelectorToUpdate, err := unstructured.NestedMap(myInterface.(map[string]interface{}), "selector")
			if err != nil {
				checkErr(err, "failed to get the 'selector' from DeploymentConfig object")
			} else if isSelectorToUpdate {
				log.Printf("Selector was found and its value is = %s", existingSelectorInterface)

				// Clean the original items tree
				unstructured.RemoveNestedField(myInterface.(map[string]interface{}), "selector")
				// Set the newest items tree
				unstructured.SetNestedMap(myInterface.(map[string]interface{}), existingSelectorInterface, "selector", "matchLabels")

				// var mSelectorKey = map[string]string{}
				// for k, v := range existingSelectorInterface {
				// 	mSelectorKey[k] = fmt.Sprint(v)
				// 	log.Printf("::: Selector key = '%+v' \n", k)
				// 	log.Printf("::: Selector value = '%+v' \n", mSelectorKey[k])
				// }

				// --- solution B --- BEGIN
				// var fixedSelector = "${APP_NAME}"
				// // updatedSelector := map[string]interface{}{
				// // updatedSelector := []interface{}{
				// updatedSelector := map[string]interface{}{
				// 	// "matchLabels": []interface{}{
				// 	"matchLabels": map[string]interface{}{
				// 		// existingSelectorInterface,
				// 		"app":              fixedSelector,
				// 		"deploymentconfig": fixedSelector,
				// 	},
				// }
				// unstructured.SetNestedStringMap(myInterface.(map[string]interface{}), updatedSelector, "selector", "matchLabels")
			}

		// ::: Route Vs Ingress :::
		case "Route":
			log.Printf("Converting the object from: %s into 'Ingress'", k8sR.GetKind())

			// ::: GET the 'Service Name' from the source Route object
			getTargetService, _, err := unstructured.NestedFieldNoCopy(k8sR.Object, "spec", "to")
			if err != nil {
				checkErr(err, "failed to get the 'service' name from the 'route' object")
			}

			var mTargetService = map[string]string{}
			for k, v := range getTargetService.(map[string]interface{}) {
				mTargetService[k] = fmt.Sprint(v)
				// check if exist
				_, ok := mTargetService["name"]
				if ok {
					log.Printf("::: Service Name = '%+v' \n", mTargetService["name"])
				}
			}

			// ::: GET the 'Target Port' from the source Route object
			getTargetPort, _, err := unstructured.NestedFieldNoCopy(k8sR.Object, "spec", "port")
			if err != nil {
				checkErr(err, "failed to get the 'target port' from the 'route' object")
			}
			var TargetPort (string)
			for _, v := range getTargetPort.(map[string]interface{}) {
				// extract port number from name- WARNING: it is a workaround to fix
				re := regexp.MustCompile(`[-]?\d[\d,]*[\.]?[\d{2}]*`)
				if !(re.MatchString(v.(string))) {
					log.Fatalf("::: Error - failed to get the service port number from route obj definition")
					// os.Exit(1)
				}
				log.Printf("::: Service Port = '%+v'\n", re.FindString(v.(string)))
				TargetPort = fmt.Sprint(re.FindString(v.(string)))
			}

			// ::: "Ingress" template without specify the ingressClassName aimed to use the default set on the cluster if any
			// ::: referring to: https://kubernetes.io/docs/concepts/services-networking/ingress/#default-ingress-class
			jsonIngressTemp := `{
				"apiVersion": "networking.k8s.io/v1",
				"kind": "Ingress",
				"metadata": {
					"name": "ingress-` + k8sR.GetName() + `",
					"annotations": {
						"nginx.ingress.kubernetes.io/rewrite-target": "/"
					}
				},
				"spec": {
					"rules": [
						{
							"http": {
								"paths": [
									{
										"path": "/",
										"pathType": "Prefix",
										"backend": {
											"service": {
												"name": "` + mTargetService["name"] + `",
												"port": {
													"number": ` + TargetPort + `
												}
											}
										}
									}
								]
							}
						}
					]
				}
			}`

			// fmt.Printf("\n ::: DEBUG - the object k8sR before overwrite :::::::::::: %s\n", k8sR)

			// ::: solution 1 - map[string]interface{} object
			var IngressObjData map[string]interface{}
			errIngressObjData := json.Unmarshal([]byte(jsonIngressTemp), &IngressObjData)
			if errIngressObjData != nil {
				checkErr(errIngressObjData, "::: Error - failed to get the 'service' name from the 'route' object")
			}

			// fmt.Printf("\n ::: DEBUG - the json map 'IngressObjData' is: %v\n", IngressObjData)

			// ::: Set the new 'Object Kind'
			k8sR.SetKind("Ingress")

			// ::: Overwrite by the new map 'Ingress object'
			k8sR.SetUnstructuredContent(IngressObjData)
		}

		labels := k8sR.GetLabels()
		if labels == nil {
			k8sR.SetLabels(*templateLabels)
		} else {
			for key, value := range *templateLabels {
				labels[key] = value
			}
			k8sR.SetLabels(labels)
		}

		updatedJSON, err := k8sR.MarshalJSON()
		if err != nil {
			return fmt.Errorf(fmt.Sprintf("Failed to marshal Unstructured record to JSON\n%v\n", k8sR) + err.Error())
		}

		log.Printf("Creating a template for object %s", k8sR.GetKind())
		data, err := yaml.JSONToYAML(updatedJSON)
		if err != nil {
			return fmt.Errorf(fmt.Sprintf("Failed to marshal Raw resource back to YAML\n%v\n", updatedJSON) + err.Error())
		}

		if m[k8sR.GetKind()] == nil {
			m[k8sR.GetKind()] = data
		} else {
			newdata := append(m[k8sR.GetKind()], separator...)
			data = append(newdata, data...)
			m[k8sR.GetKind()] = data
		}

	}

	// Create chart using map
	for k, v := range m {

		name := "templates/" + strings.ToLower(k+".yaml")

		tf := chart.File{
			Name: name,
			Data: v,
		}
		*templates = append(*templates, &tf)
	}

	return nil
}

func paramsToValues(param *[]template.Parameter, values *map[string]interface{}, templates *[]*chart.File) error {

	p := *param
	t := *templates
	v := *values

	for _, pm := range p {
		name := strings.ToLower(pm.Name)
		log.Printf("Convert parameter %s to value .%s", pm.Name, name)

		for i, tf := range t {
			// Search and replace ${PARAM} with {{ .Values.param }}
			raw := tf.Data
			// Handle string format parameters
			ns := strings.ReplaceAll(string(raw), fmt.Sprintf("${%s}", pm.Name), fmt.Sprintf("{{ .Values.%s }}", name))
			// TODO Handle binary formatted data differently
			ns = strings.ReplaceAll(ns, fmt.Sprintf("${{%s}}", pm.Name), fmt.Sprintf("{{ .Values.%s }}", name))
			ntf := chart.File{
				Name: tf.Name,
				Data: []byte(ns),
			}

			t[i] = &ntf
		}

		if pm.Value != "" {
			v[name] = pm.Value
		} else {
			v[name] = "# TODO: must define a default value for ." + name
		}
	}

	*templates = t
	*values = v

	return nil
}

// func injectEnvInDeployment(obj unstructured.Unstructured) error {

// 	newEnvs := []interface{}{
// 		map[string]interface{}{
// 			"name": "TEST_POD_UID",
// 			"valueFrom": map[string]interface{}{
// 				"fieldRef": map[string]interface{}{
// 					"fieldPath": "metadata.uid",
// 				},
// 			},
// 		},
// 	}
// 	conInterface, _, err := unstructured.NestedFieldNoCopy(obj.Object, "spec", "template", "spec", "containers")
// 	if err != nil {
// 		checkErr(err, "failed to get containers")
// 	}
// 	containers, ok := conInterface.([]interface{})
// 	if !ok {
// 		return fmt.Errorf("expected of type %T but got %T", []interface{}{}, conInterface)
// 	}
// 	existingEnvInterface, _, err := unstructured.NestedFieldNoCopy(containers[0].(map[string]interface{}), "env")
// 	if err != nil {
// 		checkErr(err, "failed to get envs present in container")
// 	}
// 	var updatedEnvs []interface{}
// 	if existingEnvInterface != nil {
// 		updatedEnvs = append(existingEnvInterface.([]interface{}), newEnvs...)
// 	} else {
// 		updatedEnvs = newEnvs
// 	}
// 	return unstructured.SetNestedField(containers[0].(map[string]interface{}), updatedEnvs, "env")
// }
