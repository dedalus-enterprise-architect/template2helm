package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
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
				return fmt.Errorf("::: ERROR - Couldn't load template: %v", err)
			}

			// Convert to json first
			jsonB, err := yaml.YAMLToJSON(yamlFile)
			checkErr(err, fmt.Sprintf("::: ERROR - Error transforming yaml to json: \n%s", string(yamlFile)))

			err = json.Unmarshal(jsonB, &myTemplate)
			checkErr(err, "::: ERROR - Unable to marshal template")

			// Convert myTemplate.Objects into individual files
			var templates []*chart.File
			err = objectToTemplate(&myTemplate.Objects, &myTemplate.ObjectLabels, &templates)
			checkErr(err, "::: ERROR - failed object to template conversion")

			// Convert myTemplate.Parameters into a yaml string map
			values := make(map[string]interface{})
			err = paramsToValues(&myTemplate.Parameters, &values, &templates)
			checkErr(err, "::: ERROR - failed parameter to value conversion")

			valuesAsByte, err := yaml.Marshal(values)
			checkErr(err, "::: ERROR - failed converting values to YAML")

			myChart := chart.Chart{
				Metadata: &chart.Metadata{
					Name:        myTemplate.ObjectMeta.Name,
					APIVersion:  "v2",
					Version:     myTemplate.ObjectMeta.Annotations["appversion"],
					AppVersion:  myTemplate.ObjectMeta.Annotations["appversion"],
					Description: myTemplate.ObjectMeta.Annotations["description"],
					// Set the factory icon:
					Icon: "data:text/plain;base64,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",
					// Tags:        myTemplate.ObjectMeta.Annotations["tags"],
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

			if myChart.Metadata.Version == "" {
				if myChart.Values["app_version"] != nil {
					myChart.Metadata.Version = fmt.Sprint(myChart.Values["app_version"])
				} else {
					myChart.Metadata.Version = "v0.0.1"
				}
				log.Printf("::: INFO - Setting the Chart 'Version': %s", myChart.Metadata.Version)
			}

			if myChart.Metadata.AppVersion == "" {
				myChart.Metadata.AppVersion = fmt.Sprint(myChart.Values["app_version"])
				if myChart.Values["app_version"] != nil {
					myChart.Metadata.AppVersion = fmt.Sprint(myChart.Values["app_version"])
				} else {
					myChart.Metadata.AppVersion = "v0.0.1"
				}
				log.Printf("::: INFO - Setting the Chart 'AppVersion': %s", myChart.Metadata.AppVersion)
			}

			err = chartutil.SaveDir(&myChart, chartPath)
			checkErr(err, fmt.Sprintf("::: ERROR - failed to save chart %s", myChart.Metadata.Name))

			// :::
			// :: OPTIONAL - adding the helm chart template about the objects which are not compliants with the object kind
			// :::

			// Ingress Objects
			object2HelmTemplate(&myChart, "/templates/ingress.yaml", "/templates/ingress.yaml")
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
	// return
}

func object2HelmTemplate(myChart *chart.Chart, srcObjectName string, targetObjectName string) error {

	objFileSrc, err := os.ReadFile(filepath.Clean(chartPath + myChart.ChartFullPath() + srcObjectName))
	if err != nil {
		// checkErr(err, fmt.Sprintf("::: ERROR - Couldn't load the Ingress object: %v", err))
		log.Printf("::: WARNING - Couldn't load the Ingress object:\n %v", err)
	}

	objFile, err := os.OpenFile(filepath.Clean(chartPath+myChart.ChartFullPath()+targetObjectName), os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("::: ERROR - Couldn't load the Ingress object: %v", err)
	}

	// write line on top of the file
	if _, err := objFile.WriteString(`{{ if not (.Capabilities.APIVersions.Has "security.openshift.io/v1/SecurityContextConstraints") }}` + "\n"); err != nil {
		log.Printf("::: ERROR - failed to add line to the chart file: %s", chartPath+myChart.ChartFullPath()+targetObjectName)
	}
	// write the whole original file
	if _, err := objFile.Write(objFileSrc); err != nil {
		log.Printf("::: ERROR - failed to add line to the chart file: %s", chartPath+myChart.ChartFullPath()+targetObjectName)
	}
	// write line on bottom of the file
	if _, err := objFile.WriteString(`{{ end }}` + "\n"); err != nil {
		log.Printf("::: ERROR - failed to add line to the chart file: %s", chartPath+myChart.ChartFullPath()+targetObjectName)
	}
	// closing the files
	if err := objFile.Close(); err != nil {
		log.Printf("::: ERROR - failed to save the chart file: %s", chartPath+myChart.ChartFullPath()+targetObjectName)
	}

	return nil
}

// Convert the object list in the openshift template to a set of template files in the chart
func objectToTemplate(objects *[]runtime.RawExtension, templateLabels *map[string]string, templates *[]*chart.File) error {
	o := *objects

	m := make(map[string][]byte)
	separator := []byte{'-', '-', '-', '\n'}

	var mServiceObj = map[int]map[string]string{} // it is needed by object kind = service

	for _, v := range o {
		var k8sR unstructured.Unstructured
		err := json.Unmarshal([]byte(v.Raw), &k8sR)
		if err != nil {
			return fmt.Errorf(fmt.Sprintf("::: ERROR - failed to unmarshal Raw resource\n%v\n", v.Raw) + err.Error())
		}

		objectKind := k8sR.GetKind()
		switch objectKind {
		// ::: DeploymentConfig Vs Deployment :::
		case "DeploymentConfig":
			log.Printf("::: INFO - Deployment - converting the object from: %s into 'Deployment'", k8sR.GetKind())
			// ::: Change the apiVersion
			log.Printf("::: INFO - Deployment - change the current apiVersion: %s ", k8sR.GetAPIVersion())
			k8sR.SetAPIVersion("apps/v1")

			// ::: Change the object kind
			log.Printf("::: INFO - Deployment - change the current object type: %s ", k8sR.GetKind())
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
			log.Printf("::: INFO - Deployment - remove the 'strategy' branch from the object: %s ", k8sR.GetKind())
			myInterface, _, err := unstructured.NestedFieldNoCopy(k8sR.Object, "spec")
			if err != nil {
				return fmt.Errorf(fmt.Sprintf("\n::: ERROR - Deployment - failed to parse the object %s with the following Error: ", k8sR.GetKind()) + err.Error())
			}
			unstructured.RemoveNestedField(myInterface.(map[string]interface{}), "strategy")
			unstructured.RemoveNestedField(myInterface.(map[string]interface{}), "test")
			unstructured.RemoveNestedField(myInterface.(map[string]interface{}), "triggers")

			//
			// Get the original selector items tree
			//
			existingSelectorMatchLabels, isSelectorExist, err := unstructured.NestedMap(myInterface.(map[string]interface{}), "selector", "matchLabels")
			if err != nil {
				checkErr(err, "::: ERROR - failed to get the 'selector.matchLabels' from DeploymentConfig object")
			} else if isSelectorExist { // if already exist jump to the next case
				log.Printf("::: INFO - Deployment - skipping the Selector because is appears as already configured = %s", existingSelectorMatchLabels)
				break
			}

			existingSelectorInterface, isSelectorToUpdate, err := unstructured.NestedMap(myInterface.(map[string]interface{}), "selector")
			if err != nil {
				checkErr(err, "::: ERROR - Deployment - failed to get the 'selector' from DeploymentConfig object")
			} else if isSelectorToUpdate {
				log.Printf("::: INFO - Deployment - selector was found and its value is = %s", existingSelectorInterface)

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

				// --- building a fixed structured interface ---
				// var fixedSelector = "${APP_NAME}"
				// updatedSelector := map[string]interface{}{
				// 	"matchLabels": map[string]interface{}{
				// 		// existingSelectorInterface,
				// 		"app":              fixedSelector,
				// 		"deploymentconfig": fixedSelector,
				// 	},
				// }
				// unstructured.SetNestedStringMap(myInterface.(map[string]interface{}), updatedSelector, "selector", "matchLabels")
			}

		case "Service":

			getServicePorts, _, err := unstructured.NestedFieldNoCopy(k8sR.Object, "spec", "ports")
			if err != nil {
				checkErr(err, "::: ERROR - Service - failed to get the 'ports' name from the 'service' object")
			}

			for key, value := range getServicePorts.([]interface{}) {
				// fmt.Printf("key = %+v\n value = %+v", key, value)
				keyy := key + len(mServiceObj)
				mServiceObj[keyy] = map[string]string{}
				for kk, vv := range value.(map[string]interface{}) {
					mServiceObj[keyy][kk] = fmt.Sprint(vv)
					// fmt.Printf("key: '%+v' and value: '%+v'", kk, vv)
				}
			}

			// for i := range getServicePorts.(map[string]interface{}) {
			// 	// for k, y := range getServicePorts.(map[string]interface{}) {

			// 		fmt.Println(getServicePorts[i])
			// 		// ServiceObj[i] = fmt.Sprint(y)
			// 		// log.Printf("::: INFO - Service Port = '%+v'\n", v.(string))
			// 		// fmt.Sprint(k)
			// 		// fmt.Sprint(v)
			// 	// }
			// }

		// ::: Route Vs Ingress :::
		case "Route":
			log.Printf("::: INFO - Route - converting the object from: %s into 'Ingress'", k8sR.GetKind())

			// ::: GET the 'Service Name' from the source Route object
			getTargetService, _, err := unstructured.NestedFieldNoCopy(k8sR.Object, "spec", "to")
			if err != nil {
				checkErr(err, "::: ERROR - Route - failed to get the 'service' name from the 'route' object")
			}

			var mTargetService = map[string]string{}
			for k, v := range getTargetService.(map[string]interface{}) {
				mTargetService[k] = fmt.Sprint(v)
				// check if exist
				_, ok := mTargetService["name"]
				if ok {
					log.Printf("::: INFO - Route - get the target service name = '%+v' \n", mTargetService["name"])
				}
			}

			// ::: GET the 'Target Port' from the source Route object
			getTargetPort, _, err := unstructured.NestedFieldNoCopy(k8sR.Object, "spec", "port", "targetPort")
			if err != nil {
				checkErr(err, "::: ERROR - Route - failed to get the 'target port' from the 'route' object")
			}

			var TargetPort (string)
			for _, srvObjV := range mServiceObj {
				if getTargetPort == srvObjV["name"] { // set the matched target port on Ingress object
					log.Printf("::: INFO - Route - finding the service port: '%+v' whose match with the target port: '%+v' \n", srvObjV["name"], srvObjV["targetPort"])
					TargetPort = fmt.Sprint(srvObjV["targetPort"])
					break
				}
			}

			// ::: extract port number from the service name
			// for _, v := range getTargetPort.(string) {
			// 	re := regexp.MustCompile(`[-]?\d[\d,]*[\.]?[\d{2}]*`)
			// 	if !(re.MatchString(v.(string))) {
			// 		log.Fatalf("::: ERROR - failed to get the service port number from route obj definition")
			// 	}
			// 	log.Printf("::: INFO - Service Port = '%+v'\n", re.FindString(v.(string)))
			// 	TargetPort = fmt.Sprint(re.FindString(v.(string)))
			// }

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

			var IngressObjData map[string]interface{}
			errIngressObjData := json.Unmarshal([]byte(jsonIngressTemp), &IngressObjData)
			if errIngressObjData != nil {
				checkErr(errIngressObjData, fmt.Sprintf("::: ERROR - Route - failed to get the 'service name': %s from the 'route' object\n", mTargetService["name"]))
			}

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
			return fmt.Errorf(fmt.Sprintf("::: ERROR - failed to marshal Unstructured record to JSON\n%v\n", k8sR) + err.Error())
		}

		log.Printf("::: INFO - Creating a template for object %s", k8sR.GetKind())
		data, err := yaml.JSONToYAML(updatedJSON)
		if err != nil {
			return fmt.Errorf(fmt.Sprintf("::: ERROR - failed to marshal Raw resource back to YAML\n%v\n", updatedJSON) + err.Error())
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
		log.Printf("::: INFO - convert parameter %s to value .%s", pm.Name, name)

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
