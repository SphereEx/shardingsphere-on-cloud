/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package reconcile

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/api/v1alpha1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewDeployment(ssproxy *v1alpha1.ShardingSphereProxy) *v1.Deployment {
	return ConstructCascadingDeployment(ssproxy)
}
func ConstructCascadingDeployment(proxy *v1alpha1.ShardingSphereProxy) *v1.Deployment {
	if proxy == nil || reflect.DeepEqual(proxy, &v1alpha1.ShardingSphereProxy{}) {
		return &v1.Deployment{}
	}

	dp := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy.Name,
			Namespace: proxy.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(proxy.GetObjectMeta(), proxy.GroupVersionKind()),
			},
		},
		Spec: v1.DeploymentSpec{
			Strategy: v1.DeploymentStrategy{
				Type: v1.RecreateDeploymentStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"apps": proxy.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"apps": proxy.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "proxy",
							Image:           fmt.Sprintf("%s:%s", imageName, proxy.Spec.Version),
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: proxy.Spec.Port,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "PORT",
									Value: strconv.FormatInt(int64(proxy.Spec.Port), 10),
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/opt/shardingsphere-proxy/conf",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: proxy.Spec.ProxyConfigName,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if proxy.Spec.AutomaticScaling == nil {
		dp.Spec.Replicas = &proxy.Spec.Replicas
	}

	dp.Spec.Template.Spec.Containers[0].Resources = proxy.Spec.Resources

	if proxy.Spec.LivenessProbe != nil {
		dp.Spec.Template.Spec.Containers[0].LivenessProbe = proxy.Spec.LivenessProbe
	}
	if proxy.Spec.ReadinessProbe != nil {
		dp.Spec.Template.Spec.Containers[0].ReadinessProbe = proxy.Spec.ReadinessProbe
	}
	if proxy.Spec.StartupProbe != nil {
		dp.Spec.Template.Spec.Containers[0].StartupProbe = proxy.Spec.StartupProbe
	}
	if len(proxy.Spec.ImagePullSecrets) > 0 {
		dp.Spec.Template.Spec.ImagePullSecrets = proxy.Spec.ImagePullSecrets
	}
	return processOptionalParameter(proxy, dp)
}

func processOptionalParameter(proxy *v1alpha1.ShardingSphereProxy, dp *v1.Deployment) *v1.Deployment {
	if proxy.Spec.MySQLDriver != nil {
		addInitContainer(dp, proxy.Spec.MySQLDriver)
	}
	return dp
}

const script = `wget https://repo1.maven.org/maven2/mysql/mysql-connector-java/${VERSION}/mysql-connector-java-${VERSION}.jar;
wget https://repo1.maven.org/maven2/mysql/mysql-connector-java/${VERSION}/mysql-connector-java-${VERSION}.jar.md5;
if [ $(md5sum /mysql-connector-java-${VERSION}.jar | cut -d ' ' -f1) = $(cat /mysql-connector-java-${VERSION}.jar.md5) ];
then echo success;
else echo failed;exit 1;fi;mv /mysql-connector-java-${VERSION}.jar /opt/shardingsphere-proxy/ext-lib`

func addInitContainer(dp *v1.Deployment, mysql *v1alpha1.MySQLDriver) {
	if len(dp.Spec.Template.Spec.InitContainers) == 0 {
		dp.Spec.Template.Spec.Containers[0].VolumeMounts = append(dp.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "mysql-connect-jar",
			MountPath: "/opt/shardingsphere-proxy/ext-lib",
		})

		dp.Spec.Template.Spec.Volumes = append(dp.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "mysql-connect-jar",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	dp.Spec.Template.Spec.InitContainers = []corev1.Container{
		{
			Name:    "download-mysql-connect",
			Image:   "busybox:1.35.0",
			Command: []string{"/bin/sh", "-c", script},
			Env: []corev1.EnvVar{
				{
					Name:  "VERSION",
					Value: mysql.Version,
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "mysql-connect-jar",
					MountPath: "/opt/shardingsphere-proxy/ext-lib",
				},
			},
		},
	}

}

// UpdateDeployment FIXME:merge UpdateDeployment and ConstructCascadingDeployment
func UpdateDeployment(proxy *v1alpha1.ShardingSphereProxy, act *v1.Deployment) *v1.Deployment {
	exp := act.DeepCopy()

	if proxy.Spec.AutomaticScaling == nil || !proxy.Spec.AutomaticScaling.Enable {
		exp.Spec.Replicas = updateReplicas(proxy, act)
	}
	exp.Spec.Template = updatePodTemplateSpec(proxy, act)
	return exp
}

func updateReplicas(proxy *v1alpha1.ShardingSphereProxy, act *v1.Deployment) *int32 {
	if *act.Spec.Replicas != proxy.Spec.Replicas {
		return &proxy.Spec.Replicas
	}
	return act.Spec.Replicas
}

func updatePodTemplateSpec(proxy *v1alpha1.ShardingSphereProxy, act *v1.Deployment) corev1.PodTemplateSpec {
	exp := act.Spec.Template.DeepCopy()

	SSProxyContainer := updateSSProxyContainer(proxy, act)
	for i, _ := range exp.Spec.Containers {
		if exp.Spec.Containers[i].Name == "proxy" {
			exp.Spec.Containers[i] = *SSProxyContainer
		}
	}

	initContainer := updateInitContainer(proxy, act)
	for i, _ := range exp.Spec.InitContainers {
		if exp.Spec.InitContainers[i].Name == "download-mysql-connect" {
			exp.Spec.InitContainers[i] = *initContainer
		}
	}

	configName := updateConfigName(proxy, act)
	exp.Spec.Volumes[0].ConfigMap.Name = configName

	return *exp
}

func updateConfigName(proxy *v1alpha1.ShardingSphereProxy, act *v1.Deployment) string {
	if act.Spec.Template.Spec.Volumes[0].ConfigMap.Name != proxy.Spec.ProxyConfigName {
		return proxy.Spec.ProxyConfigName
	}
	return act.Spec.Template.Spec.Volumes[0].ConfigMap.Name
}

func updateInitContainer(proxy *v1alpha1.ShardingSphereProxy, act *v1.Deployment) *corev1.Container {
	var exp *corev1.Container

	for _, c := range act.Spec.Template.Spec.InitContainers {
		if c.Name == "download-mysql-connect" {
			for i, _ := range c.Env {
				if c.Env[i].Name == "VERSION" {
					if c.Env[i].Value != proxy.Spec.MySQLDriver.Version {
						c.Env[i].Value = proxy.Spec.MySQLDriver.Version
					}
				}
			}
			exp = c.DeepCopy()
		}
	}

	return exp
}

func updateSSProxyContainer(proxy *v1alpha1.ShardingSphereProxy, act *v1.Deployment) *corev1.Container {
	var exp *corev1.Container

	for _, c := range act.Spec.Template.Spec.Containers {
		if c.Name == "proxy" {
			exp = c.DeepCopy()

			tag := strings.Split(c.Image, ":")[1]
			if tag != proxy.Spec.Version {
				exp.Image = fmt.Sprintf("%s:%s", imageName, proxy.Spec.Version)
			}

			exp.Resources = proxy.Spec.Resources

			if proxy.Spec.LivenessProbe != nil && !reflect.DeepEqual(c.LivenessProbe, *proxy.Spec.LivenessProbe) {
				exp.LivenessProbe = proxy.Spec.LivenessProbe
			}

			if proxy.Spec.ReadinessProbe != nil && !reflect.DeepEqual(c.ReadinessProbe, *proxy.Spec.ReadinessProbe) {
				exp.ReadinessProbe = proxy.Spec.ReadinessProbe
			}

			if proxy.Spec.StartupProbe != nil && !reflect.DeepEqual(c.StartupProbe, *proxy.Spec.StartupProbe) {
				exp.StartupProbe = proxy.Spec.StartupProbe
			}

			for i, _ := range c.Env {
				if c.Env[i].Name == "PORT" {
					proxyPort := strconv.FormatInt(int64(proxy.Spec.Port), 10)
					if c.Env[i].Value != proxyPort {
						c.Env[i].Value = proxyPort
						exp.Ports[0].ContainerPort = proxy.Spec.Port
					}
				}
			}
			exp.Env = c.Env
		}
	}
	return exp
}
