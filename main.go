/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/prometheus/common/version"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func main() {
	zapOpts := &zap.Options{}
	opts := &options{}
	zapOpts.BindFlags(flag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	opts.BindFlags(pflag.CommandLine)
	certDir := pflag.String("certdir", filepath.Join(os.TempDir(), "k8s-webhook-server", "serving-certs"), "The directory that contains the server key and certificate")
	printVersionInfo := pflag.BoolP("version", "v", false, "Print version info")
	pflag.Parse()

	if *printVersionInfo {
		fmt.Println(version.Print("kubeimageswap"))
		os.Exit(0)
	}

	log.SetLogger(zap.New(zap.UseFlagOptions(zapOpts)))

	setupLog := log.Log.WithName("setup")

	// Setup a Manager
	setupLog.Info("setting up manager")
	cfg, err := config.GetConfig()
	if err != nil {
		// webhook does not need kubeconfig
		cfg = &rest.Config{}
	}
	mgr, err := manager.New(cfg, manager.Options{
		WebhookServer: webhook.NewServer(webhook.Options{CertDir: *certDir}),
	})
	if err != nil {
		setupLog.Error(err, "unable to set up overall manager")
		os.Exit(1)
	}

	defaulter, err := newDefaulter(opts)
	if err != nil {
		setupLog.Error(err, "unable to create defaulter")
		os.Exit(1)
	}

	if err := builder.WebhookManagedBy(mgr).
		For(&corev1.Pod{}).
		WithDefaulter(defaulter).
		Complete(); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Pod")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "unable to run manager")
		os.Exit(1)
	}
}
