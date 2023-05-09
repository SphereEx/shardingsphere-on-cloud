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

package controllers_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/api/v1alpha1"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/cmd/shardingsphere-operator/manager"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/controllers"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/configmap"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/deployment"
	"github.com/apache/shardingsphere-on-cloud/shardingsphere-operator/pkg/kubernetes/service"

	dbmesh_aws "github.com/database-mesh/golang-sdk/aws"
	dbmesh_rds "github.com/database-mesh/golang-sdk/aws/client/rds"
	dbmeshv1alpha1 "github.com/database-mesh/golang-sdk/kubernetes/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	cfg       *rest.Config
	k8sClient client.Client // You'll be using this client in your tests.
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controllers Suite")
}

func loadOnlineCRDs() {
	urls := []string{
		// DatabaseClass CRD file
		"https://raw.githubusercontent.com/database-mesh/golang-sdk/main/config/crd/bases/core.database-mesh.io_databaseclasses.yaml",
	}

	filePath := filepath.Join("..", "..", "config", "crd", "bases")
	for _, url := range urls {
		Expect(exec.Command("wget", url, "-nc", "-P", filePath).Run()).Should(Succeed())
	}
}

var _ = BeforeSuite(func() {
	loadOnlineCRDs()

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment for controllers")

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
		CRDInstallOptions: envtest.CRDInstallOptions{
			MaxTime: 60 * time.Second,
		},
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(v1alpha1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	Expect(dbmeshv1alpha1.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())
	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// set metrics bind address to :9081, diff from default metric port:8080 and health check port:8081 to avoid conflict port when running tests
	os.Args = append(os.Args, "--metrics-bind-address=:9081")
	opt := manager.ParseOptionsFromCmdFlags()

	k8sManager, err := ctrl.NewManager(cfg, opt.Options)
	Expect(err).ToNot(HaveOccurred())
	// print k8sManager Options
	sess := dbmesh_aws.NewSessions().SetCredential("AwsRegion", "AwsAccessKeyID", "AwsSecretAccessKey").Build()
	err = (&controllers.StorageNodeReconciler{
		Client:   k8sManager.GetClient(),
		Scheme:   k8sManager.GetScheme(),
		Log:      ctrl.Log.WithName("controllers").WithName("StorageNode"),
		Recorder: k8sManager.GetEventRecorderFor("StorageNode"),
		AwsRDS:   dbmesh_rds.NewService(sess["AwsRegion"]),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	err = (&controllers.ComputeNodeReconciler{
		Client:     k8sManager.GetClient(),
		Scheme:     k8sManager.GetScheme(),
		Log:        logf.Log.WithName("controllers").WithName("ComputeNode"),
		Deployment: deployment.NewDeploymentClient(k8sManager.GetClient()),
		Service:    service.NewServiceClient(k8sManager.GetClient()),
		ConfigMap:  configmap.NewConfigMapClient(k8sManager.GetClient()),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())
	/*

		ctl := gomock.NewController(GinkgoT())
		clientset, err := clientset.NewForConfig(k8sManager.GetConfig())
		err = (&controllers.ShardingSphereChaosReconciler{
			Client:    k8sManager.GetClient(),
			Scheme:    k8sManager.GetScheme(),
			Log:       logf.Log,
			Chaos:     mockChaos.NewMockChaos(ctl),
			Job:       job.NewJob(k8sManager.GetClient()),
			ConfigMap: configmap.NewConfigMapClient(k8sManager.GetClient()),
			Events:    k8sManager.GetEventRecorderFor("shardingsphere-chaos-controller"),
			ClientSet: clientset,
		}).SetupWithManager(k8sManager)
		Expect(err).ToNot(HaveOccurred())
	*/

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment for controllers")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
