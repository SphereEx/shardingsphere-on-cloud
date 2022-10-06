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
	v1 "k8s.io/api/core/v1"
)

func IsRunning(podList *v1.PodList) bool {
	status := false
	for _, pod := range podList.Items {
		if pod.Status.Phase == v1.PodRunning && pod.ObjectMeta.DeletionTimestamp == nil {
			status = true
		}
	}

	return status
}

func CountingReadyPods(podList *v1.PodList) int32 {
	var readyPods int32 = 0
	for _, pod := range podList.Items {
		if len(pod.Status.ContainerStatuses) == 0 {
			continue
		}
		if pod.Status.ContainerStatuses[0].Ready && pod.ObjectMeta.DeletionTimestamp == nil {
			readyPods++
		}
	}
	return readyPods
}
