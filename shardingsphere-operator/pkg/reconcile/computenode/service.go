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

package computenode

import (
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func NewService(cn *v1alpha1.ComputeNode) *v1.Service {
	builder := NewServiceBuilder(cn.GetObjectMeta(), cn.GetObjectKind().GroupVersionKind())
	builder.SetName(cn.Name).SetNamespace(cn.Namespace).SetLabelsAndSelectors(cn.Labels, cn.Spec.Selector).SetAnnotations(cn.Annotations).SetType(cn.Spec.ServiceType)

	ports := []v1.ServicePort{}
	for _, pb := range cn.Spec.PortBindings {
		ports = append(ports, corev1.ServicePort{
			Name:       pb.Name,
			Port:       pb.ServicePort,
			TargetPort: intstr.FromInt(int(pb.ContainerPort)),
			Protocol:   pb.Protocol,
		})
	}
	builder.SetPorts(ports)
	return builder.Build()
}

type ServiceBuilder interface {
	SetName(name string) ServiceBuilder
	SetNamespace(namespace string) ServiceBuilder
	SetLabelsAndSelectors(labels map[string]string, selectors *metav1.LabelSelector) ServiceBuilder
	SetAnnotations(anno map[string]string) ServiceBuilder
	SetType(t corev1.ServiceType) ServiceBuilder
	SetPorts(ports []corev1.ServicePort) ServiceBuilder
	Build() *corev1.Service
}

func NewServiceBuilder(meta metav1.Object, gvk schema.GroupVersionKind) ServiceBuilder {
	return &serviceBuilder{
		service: DefaultService(meta, gvk),
	}
}

type serviceBuilder struct {
	service *corev1.Service
}

func (s *serviceBuilder) SetName(name string) ServiceBuilder {
	s.service.Name = name
	return s
}

func (s *serviceBuilder) SetNamespace(namespace string) ServiceBuilder {
	s.service.Namespace = namespace
	return s
}

func (s *serviceBuilder) SetLabelsAndSelectors(labels map[string]string, selectors *metav1.LabelSelector) ServiceBuilder {
	s.service.Labels = labels
	s.service.Spec.Selector = selectors.MatchLabels
	return s
}

func (s *serviceBuilder) SetAnnotations(annos map[string]string) ServiceBuilder {
	s.service.Annotations = annos
	return s
}

func (s *serviceBuilder) SetType(t corev1.ServiceType) ServiceBuilder {
	s.service.Spec.Type = t
	return s
}

func (s *serviceBuilder) SetPorts(ports []corev1.ServicePort) ServiceBuilder {
	if s.service.Spec.Ports == nil {
		s.service.Spec.Ports = []v1.ServicePort{}
	}
	s.service.Spec.Ports = ports
	return s
}

func (s *serviceBuilder) Build() *corev1.Service {
	return s.service
}

func DefaultService(meta metav1.Object, gvk schema.GroupVersionKind) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shardingsphere-proxy",
			Namespace: "default",
			Labels:    map[string]string{},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(meta, gvk),
			},
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{},
			Type:     v1.ServiceTypeClusterIP,
		},
	}
}

func UpdateService(cn *v1alpha1.ComputeNode, cur *v1.Service) *v1.Service {
	exp := &v1.Service{}
	exp.ObjectMeta = cur.ObjectMeta
	exp.Labels = cur.Labels
	exp.Annotations = cur.Annotations
	exp.Spec = NewService(cn).Spec
	exp.Spec.ClusterIP = cur.Spec.ClusterIP
	exp.Spec.ClusterIPs = cur.Spec.ClusterIPs
	if cn.Spec.ServiceType == corev1.ServiceTypeNodePort {
		for pb := range cn.Spec.PortBindings {
			for p := range cur.Spec.Ports {
				if cn.Spec.PortBindings[pb].Name == cur.Spec.Ports[p].Name {
					if cur.Spec.Ports[p].NodePort != 0 {
						for pt := range exp.Spec.Ports {
							if exp.Spec.Ports[pt].Name == cur.Spec.Ports[p].Name {
								exp.Spec.Ports[pt].NodePort = cur.Spec.Ports[p].NodePort
							}
						}
					}
				}
			}
		}
	}

	return exp
}
