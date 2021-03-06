// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package servicecmd

import (
	"flag"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/uber/cherami-server/clients/metadata"
	"github.com/uber/cherami-server/common"
	"github.com/uber/cherami-server/common/configure"
	"github.com/uber/cherami-server/common/dconfigclient"
	"github.com/uber/cherami-server/services/controllerhost"
	"github.com/uber/cherami-server/services/frontendhost"
	"github.com/uber/cherami-server/services/inputhost"
	"github.com/uber/cherami-server/services/outputhost"
	"github.com/uber/cherami-server/services/replicator"
	"github.com/uber/cherami-server/services/storehost"
	m "github.com/uber/cherami-thrift/.generated/go/metadata"

	"github.com/pborman/uuid"

	_ "net/http/pprof" //to use the side affect of pprof
)

const (
	// This is needed, since we can't listen on the same GetListenAddress() interface with two service handlers.
	// Previously, we had listened on 127.0.0.1 for one service, and the local IP for the other.
	// All of these ports happen to be reserved for our use, in any case.
	// Unlike an ephemeral port, this gives us a predictable port for our diagnostic interface
	diagnosticPortOffset = 10000
)

//StartInputHostService starts the inputhost service of cherami
func StartInputHostService() {
	serviceName := common.InputServiceName
	cfg := common.SetupServerConfig(configure.NewCommonConfigure())
	if e := os.Setenv("port", fmt.Sprintf("%d", cfg.GetServiceConfig(serviceName).GetPort())); e != nil {
		log.Panic(e)
	}

	meta, err := metadata.NewCassandraMetadataService(cfg.GetMetadataConfig())
	if err != nil {
		log.WithField(common.TagErr, err).Fatal(`inputhost: unable to instantiate metadata client`)
	}

	hwInfoReader := common.NewHostHardwareInfoReader(meta)
	reporter := common.NewMetricReporterWithHostname(cfg.GetServiceConfig(serviceName))
	dClient := dconfigclient.NewDconfigClient(cfg.GetServiceConfig(serviceName), serviceName)

	sCommon := common.NewService(serviceName, uuid.New(), cfg.GetServiceConfig(serviceName), common.NewUUIDResolver(meta), hwInfoReader, reporter, dClient, common.NewBypassAuthManager())
	h, tc := inputhost.NewInputHost(serviceName, sCommon, meta, nil)
	h.Start(tc)

	// start websocket server
	common.WSStart(cfg.GetServiceConfig(serviceName).GetListenAddress().String(),
		cfg.GetServiceConfig(serviceName).GetWebsocketPort(), h)

	// start diagnosis local http server
	common.ServiceLoop(cfg.GetServiceConfig(serviceName).GetPort()+diagnosticPortOffset, cfg, h)
}

//StartControllerService starts the controller service of cherami
func StartControllerService() {
	serviceName := common.ControllerServiceName
	cfg := common.SetupServerConfig(configure.NewCommonConfigure())
	if e := os.Setenv("port", fmt.Sprintf("%d", cfg.GetServiceConfig(serviceName).GetPort())); e != nil {
		log.Panic(e)
	}

	meta, err := metadata.NewCassandraMetadataService(cfg.GetMetadataConfig())
	if err != nil {
		// no metadata service - just fail early
		log.WithField(common.TagErr, err).Fatal(`unable to instantiate metadata service (did you run ./scripts/setup_cassandra_schema.sh?)`)
	}
	hwInfoReader := common.NewHostHardwareInfoReader(meta)
	reporter := common.NewMetricReporterWithHostname(cfg.GetServiceConfig(serviceName))
	dClient := dconfigclient.NewDconfigClient(cfg.GetServiceConfig(serviceName), serviceName)
	sVice := common.NewService(serviceName, uuid.New(), cfg.GetServiceConfig(serviceName), common.NewUUIDResolver(meta), hwInfoReader, reporter, dClient, common.NewBypassAuthManager())
	mcp, tc := controllerhost.NewController(cfg, sVice, meta, common.NewDummyZoneFailoverManager())
	mcp.Start(tc)
	common.ServiceLoop(cfg.GetServiceConfig(serviceName).GetPort()+diagnosticPortOffset, cfg, mcp.Service)
}

//StartFrontendHostService starts the frontendhost service of cherami
func StartFrontendHostService() {
	serviceName := common.FrontendServiceName
	cfg := common.SetupServerConfig(configure.NewCommonConfigure())
	if e := os.Setenv("port", fmt.Sprintf("%d", cfg.GetServiceConfig(serviceName).GetPort())); e != nil {
		log.Panic(e)
	}

	meta, err := metadata.NewCassandraMetadataService(cfg.GetMetadataConfig())
	if err != nil {
		// no metadata service - just fail early
		log.WithField(common.TagErr, err).Fatal(`frontendhost: unable to instantiate metadata service`)
	}

	hwInfoReader := common.NewHostHardwareInfoReader(meta)
	reporter := common.NewMetricReporterWithHostname(cfg.GetServiceConfig(serviceName))
	dClient := dconfigclient.NewDconfigClient(cfg.GetServiceConfig(serviceName), serviceName)
	sCommon := common.NewService(serviceName, uuid.New(), cfg.GetServiceConfig(serviceName), common.NewUUIDResolver(meta), hwInfoReader, reporter, dClient, common.NewBypassAuthManager())
	h, tc := frontendhost.NewFrontendHost(serviceName, sCommon, meta, cfg)

	// frontend host also exposes non-streaming metadata methods
	tc = append(tc, m.NewTChanMetadataExposableServer(meta))
	h.Start(tc)
	common.ServiceLoop(cfg.GetServiceConfig(serviceName).GetPort()+diagnosticPortOffset, cfg, sCommon)
}

//StartOutputHostService starts the outputhost service of cherami
func StartOutputHostService() {
	serviceName := common.OutputServiceName
	cfg := common.SetupServerConfig(configure.NewCommonConfigure())
	if e := os.Setenv("port", fmt.Sprintf("%d", cfg.GetServiceConfig(serviceName).GetPort())); e != nil {
		log.Panic(e)
	}

	meta, err := metadata.NewCassandraMetadataService(cfg.GetMetadataConfig())
	if err != nil {
		// no metadata service - just fail early
		log.WithField(common.TagErr, err).Fatal(`frontendhost: unable to instantiate metadata service`)
	}

	hwInfoReader := common.NewHostHardwareInfoReader(meta)
	reporter := common.NewMetricReporterWithHostname(cfg.GetServiceConfig(serviceName))
	dClient := dconfigclient.NewDconfigClient(cfg.GetServiceConfig(serviceName), serviceName)
	sCommon := common.NewService(serviceName, uuid.New(), cfg.GetServiceConfig(serviceName), common.NewUUIDResolver(meta), hwInfoReader, reporter, dClient, common.NewBypassAuthManager())

	// Instantiate a frontend server. Don't call frontendhost.Start(), since that would advertise in Hyperbahn,
	// and since we aren't using thrift anyway. We are selfish with our Frontend.
	frontendhost, _ := frontendhost.NewFrontendHost(common.FrontendServiceName, sCommon, meta, cfg)

	h, tc := outputhost.NewOutputHost(serviceName, sCommon, meta, frontendhost, nil, cfg.GetKafkaConfig())
	h.Start(tc)

	// start websocket server
	common.WSStart(cfg.GetServiceConfig(serviceName).GetListenAddress().String(),
		cfg.GetServiceConfig(serviceName).GetWebsocketPort(), h)

	// start diagnosis local http server
	common.ServiceLoop(cfg.GetServiceConfig(serviceName).GetPort()+diagnosticPortOffset, cfg, sCommon)
}

//StartStoreHostService starts the storehost service of cherami
func StartStoreHostService() {
	serviceName := common.StoreServiceName
	cfg := common.SetupServerConfig(configure.NewCommonConfigure())
	if e := os.Setenv("port", fmt.Sprintf("%d", cfg.GetServiceConfig(serviceName).GetPort())); e != nil {
		log.Panic(e)
	}

	meta, err := metadata.NewCassandraMetadataService(cfg.GetMetadataConfig())
	if err != nil {
		log.WithField(common.TagErr, err).Fatal(`storehost: unable to instantiate metadata client`)
	}

	hwInfoReader := common.NewHostHardwareInfoReader(meta)
	reporter := common.NewMetricReporterWithHostname(cfg.GetServiceConfig(serviceName))
	dClient := dconfigclient.NewDconfigClient(cfg.GetServiceConfig(serviceName), serviceName)
	sCommon := common.NewService(serviceName, cfg.GetStorageConfig().GetHostUUID(), cfg.GetServiceConfig(serviceName), common.NewUUIDResolver(meta), hwInfoReader, reporter, dClient, common.NewBypassAuthManager())

	// parse args and pass them into NewStoreHost
	var storeStr, baseDir string

	flag.StringVar(&storeStr, "store", cfg.GetStorageConfig().GetStore(), "store to use")
	flag.StringVar(&baseDir, "dir", "", "base directory for storage")
	flag.Parse()

	opts := &storehost.Options{BaseDir: baseDir}

	switch storeStr = strings.ToLower(storeStr); {
	case strings.Contains(storeStr, "rockstor"):
		opts.Store = storehost.Rockstor

	case strings.Contains(storeStr, "chunky"):
		opts.Store = storehost.Chunky

	case strings.Contains(storeStr, "manyrocks"):
		opts.Store = storehost.ManyRocks

	case strings.Contains(storeStr, "rockcfstor"):
		opts.Store = storehost.RockCFstor

	default:
		// don't set a default here; leave it to storehost
	}

	// BaseDir will be set from one of the following (in order):
	switch {
	case baseDir != "": // 1. if specified as command-line arg
		opts.BaseDir = baseDir

	case os.Getenv("CHERAMI_STORE") != "": // 2. if set in env-var
		opts.BaseDir = os.Getenv("CHERAMI_STORE")

	case cfg.GetStorageConfig().GetBaseDir() != "": // 3. yaml config "StorageConfig.BaseDir"
		opts.BaseDir = cfg.GetStorageConfig().GetBaseDir()

	default:
		// if none of the above, let storehost pick default
	}

	// initialize and start storehost
	h, tc := storehost.NewStoreHost(serviceName, sCommon, meta, opts)

	h.Start(tc)

	// start websocket server
	common.WSStart(cfg.GetServiceConfig(serviceName).GetListenAddress().String(),
		cfg.GetServiceConfig(serviceName).GetWebsocketPort(), h)

	// start diagnosis local http server
	common.ServiceLoop(cfg.GetServiceConfig(serviceName).GetPort()+diagnosticPortOffset, cfg, sCommon)
}

//StartReplicatorService starts the repliator service of cherami
func StartReplicatorService() {
	serviceName := common.ReplicatorServiceName
	cfg := common.SetupServerConfig(configure.NewCommonConfigure())
	if e := os.Setenv("port", fmt.Sprintf("%d", cfg.GetServiceConfig(serviceName).GetPort())); e != nil {
		log.Panic(e)
	}

	meta, err := metadata.NewCassandraMetadataService(cfg.GetMetadataConfig())
	if err != nil {
		// no metadata service - just fail early
		log.WithField(common.TagErr, err).Fatal(`frontendhost: unable to instantiate metadata service`)
	}
	hwInfoReader := common.NewHostHardwareInfoReader(meta)
	reporter := common.NewMetricReporterWithHostname(cfg.GetServiceConfig(serviceName))
	dClient := dconfigclient.NewDconfigClient(cfg.GetServiceConfig(serviceName), serviceName)
	sCommon := common.NewService(serviceName, uuid.New(), cfg.GetServiceConfig(serviceName), common.NewUUIDResolver(meta), hwInfoReader, reporter, dClient, common.NewBypassAuthManager())

	h, tc := replicator.NewReplicator(serviceName, sCommon, meta, replicator.NewReplicatorClientFactory(cfg, common.GetDefaultLogger()), cfg)
	h.Start(tc)

	// start websocket server
	common.WSStart(cfg.GetServiceConfig(serviceName).GetListenAddress().String(),
		cfg.GetServiceConfig(serviceName).GetWebsocketPort(), h)

	// start diagnosis local http server
	common.ServiceLoop(cfg.GetServiceConfig(serviceName).GetPort()+diagnosticPortOffset, cfg, sCommon)
}
