// SPDX-FileCopyrightText: 2020-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package ricapie2

import (
	"context"
	"fmt"
	"github.com/onosproject/onos-api/go/onos/e2sub/subscription"
	"github.com/onosproject/onos-e2-sm/servicemodels/e2sm_kpm_v2/pdubuilder"
	e2sm_kpm_v2 "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_kpm_v2/v2/e2sm-kpm-v2"
	"github.com/onosproject/onos-e2t/pkg/southbound/e2ap/types"
	"github.com/onosproject/onos-kpimon/pkg/southbound/admin"
	"github.com/onosproject/onos-ric-sdk-go/pkg/config/event"
	e2client "github.com/onosproject/onos-ric-sdk-go/pkg/e2"
	"github.com/onosproject/onos-ric-sdk-go/pkg/e2/indication"
	"google.golang.org/protobuf/proto"
	"strconv"
	"strings"
	"sync"
	"time"
)

// KpmServiceModelOIDV2 is the OID for KPM V2.0
const KpmServiceModelOIDV2 = "1.3.6.1.4.1.53148.1.2.2.2"

func newV2E2Session(e2tEndpoint string, e2subEndpoint string, ricActionID int32, reportPeriodMs uint64, smName string, smVersion string, kpimonMetricMap map[int]string) *V2E2Session {
	log.Info("Creating RICAPI E2Session for KPM v2.0")
	return &V2E2Session{
		AbstractE2Session: &AbstractE2Session{
			E2SubEndpoint:   e2subEndpoint,
			E2TEndpoint:     e2tEndpoint,
			RicActionID:     types.RicActionID(ricActionID),
			ReportPeriodMs:  reportPeriodMs,
			SMName:          smName,
			SMVersion:       smVersion,
			KpiMonMetricMap: kpimonMetricMap,
		},
	}
}

// V2E2Session is an E2 session for KPM v2.0
type V2E2Session struct {
	*AbstractE2Session
}

// Run starts E2 session
func (s *V2E2Session) Run(indChan chan indication.Indication, adminSession admin.E2AdminSession) {
	log.Info("Started KPIMON Southbound session")
	s.ConfigEventCh = make(chan event.Event)
	go func() {
		_ = s.watchConfigChanges()
	}()
	s.SubDelTrigger = make(chan bool)
	s.manageConnections(indChan, adminSession)
}

func (s *V2E2Session) manageConnections(indChan chan indication.Indication, adminSession admin.E2AdminSession) {
	for {
		nodeIDs, err := adminSession.GetListE2NodeIDs()
		if err != nil {
			log.Errorf("Cannot get NodeIDs through Admin API: %s", err)
			continue
		} else if len(nodeIDs) == 0 {
			log.Warn("CU-CP is not running - wait until CU-CP is ready")
			time.Sleep(1000 * time.Millisecond)
			continue
		}
		log.Infof("Received E2Nodes: %v", nodeIDs)

		hasKpiMonMetricMap := true
		for _, id := range nodeIDs {
			ranFuncDesc, err := s.getRanFuncDesc(id, adminSession)
			if err != nil {
				hasKpiMonMetricMap = false
				break
			}
			for range ranFuncDesc.GetRicKpmNodeList()[0].GetCellMeasurementObjectList() {
				for _, measInfoActionItem := range ranFuncDesc.GetRicReportStyleList()[0].GetMeasInfoActionList().GetValue() {
					actionName := measInfoActionItem.GetMeasName()
					actionID := measInfoActionItem.GetMeasId()
					log.Debugf("Check RAN function description to make KpiMonMetricMap - ranFuncDesc: %v", ranFuncDesc)
					log.Debugf("Check MeasInfoActionItem to make KpiMonMetricMap - ranFuncDesc: %v", measInfoActionItem)
					log.Debugf("KpiMonMetricMap generation - name:%v, id:%d", actionName, actionID)
					s.KpiMonMetricMap[int(actionID.Value)] = actionName.Value
				}
			}
		}

		log.Debugf("KPIMONMetricMap: %v", s.KpiMonMetricMap)

		if !hasKpiMonMetricMap {
			continue
		}

		var wg sync.WaitGroup
		for _, id := range nodeIDs {
			wg.Add(1)
			go func(id string, wg *sync.WaitGroup) {
				defer wg.Done()
				for {

					ranFuncDesc, err := s.getRanFuncDesc(id, adminSession)
					if err != nil {
						log.Error(err)
						time.Sleep(1 * time.Second)
					}
					s.manageConnection(indChan, id, ranFuncDesc)
				}
			}(id, &wg)
		}
		wg.Wait()
	}
}

func (s *V2E2Session) getRanFuncDesc(nodeID string, adminSession admin.E2AdminSession) (*e2sm_kpm_v2.E2SmKpmRanfunctionDescription, error) {
	ranFunctions, err := adminSession.GetRANFunctions(nodeID)
	if err != nil {
		return nil, err
	}

	ranFunctionDesc := &e2sm_kpm_v2.E2SmKpmRanfunctionDescription{}
	ranFunctionFound := false
	for _, ranFunction := range ranFunctions {
		if ranFunction.Oid == KpmServiceModelOIDV2 {
			err = proto.Unmarshal(ranFunction.Description, ranFunctionDesc)
			if err != nil {
				return nil, err
			}
			ranFunctionFound = true
		}
	}
	if !ranFunctionFound {
		return nil, fmt.Errorf("Cannot find RANFunction OID (%s) from nodeID %s", KpmServiceModelOIDV2, nodeID)
	}

	log.Debugf("RANFunctionDesc: %v", ranFunctionDesc)
	return ranFunctionDesc, nil
}

func (s *V2E2Session) getCellObjectID(cellMeasureObjectItem *e2sm_kpm_v2.CellMeasurementObjectItem) string {
	return cellMeasureObjectItem.CellObjectId.Value
}

func (s *V2E2Session) createActionDefinition(ranFuncDesc *e2sm_kpm_v2.E2SmKpmRanfunctionDescription) (map[string]*e2sm_kpm_v2.E2SmKpmActionDefinition, error) {
	result := make(map[string]*e2sm_kpm_v2.E2SmKpmActionDefinition)
	for _, cellMeasObjItem := range ranFuncDesc.GetRicKpmNodeList()[0].GetCellMeasurementObjectList() {
		cellObjID := s.getCellObjectID(cellMeasObjItem)
		measInfoList := &e2sm_kpm_v2.MeasurementInfoList{
			Value: make([]*e2sm_kpm_v2.MeasurementInfoItem, 0),
		}
		for _, measInfoActionItem := range ranFuncDesc.GetRicReportStyleList()[0].GetMeasInfoActionList().GetValue() {
			// for test with name
			actionName := measInfoActionItem.GetMeasName()
			tmpMeasTypeMeasName, err := pdubuilder.CreateMeasurementTypeMeasName(actionName.Value)
			if err != nil {
				return nil, err
			}

			tmpMeasInfoItem1, err := pdubuilder.CreateMeasurementInfoItem(tmpMeasTypeMeasName, nil)
			if err != nil {
				return nil, err
			}
			measInfoList.Value = append(measInfoList.Value, tmpMeasInfoItem1)

			// for test with ID
			//actionID := measInfoActionItem.GetMeasId()
			//tmpMeasTypeMeasID, err := pdubuilder.CreateMeasurementTypeMeasID(actionID.Value)
			//if err != nil {
			//	return nil, err
			//}
			//tmpMeasInfoItem2, err := pdubuilder.CreateMeasurementInfoItem(tmpMeasTypeMeasID, nil)
			//if err != nil {
			//	return nil, err
			//}
			//measInfoList.Value = append(measInfoList.Value, tmpMeasInfoItem2)
		}
		subsIDString := fmt.Sprintf("%d%d", s.RicActionID, cellMeasObjItem.GetCellGlobalId().GetEUtraCgi().EUtracellIdentity.Value.Value)
		subID, err := strconv.ParseInt(subsIDString, 10, 64)
		if err != nil {
			return nil, err
		}

		actionDefinitionCell, err := pdubuilder.CreateActionDefinitionFormat1(cellObjID, measInfoList, int32(s.GranularityMs), subID)
		if err != nil {
			return nil, err
		}

		e2smKpmActionDefinitionCell, err := pdubuilder.CreateE2SmKpmActionDefinitionFormat1(1, actionDefinitionCell)
		if err != nil {
			return nil, err
		}

		result[cellObjID] = e2smKpmActionDefinitionCell
	}
	log.Debugf("ActionDefinitions: %v", result)
	return result, nil
}

func (s *V2E2Session) manageConnection(indChan chan indication.Indication, nodeID string, ranFuncDesc *e2sm_kpm_v2.E2SmKpmRanfunctionDescription) {
	err := s.createE2Subscription(indChan, nodeID, ranFuncDesc)
	if err != nil {
		log.Errorf("Error happens when creating E2 subscription - %s", err)
	}
}

func (s *V2E2Session) createSubscriptionRequestWithActionDefinition(nodeID string, actionDef *e2sm_kpm_v2.E2SmKpmActionDefinition) (subscription.SubscriptionDetails, error) {
	protoBytesCell, err := proto.Marshal(actionDef)
	if err != nil {
		return subscription.SubscriptionDetails{}, err
	}
	sub := subscription.SubscriptionDetails{
		E2NodeID: subscription.E2NodeID(nodeID),
		ServiceModel: subscription.ServiceModel{
			Name:    subscription.ServiceModelName(s.SMName),
			Version: subscription.ServiceModelVersion(s.SMVersion),
		},
		EventTrigger: subscription.EventTrigger{
			Payload: subscription.Payload{
				Encoding: subscription.Encoding_ENCODING_PROTO,
				Data:     s.createEventTriggerData(),
			},
		},
		Actions: []subscription.Action{
			{
				ID:   int32(s.RicActionID),
				Type: subscription.ActionType_ACTION_TYPE_REPORT,
				SubsequentAction: &subscription.SubsequentAction{
					Type:       subscription.SubsequentActionType_SUBSEQUENT_ACTION_TYPE_CONTINUE,
					TimeToWait: subscription.TimeToWait_TIME_TO_WAIT_ZERO,
				},
				Payload: subscription.Payload{
					Encoding: subscription.Encoding_ENCODING_PROTO,
					Data:     protoBytesCell,
				},
			},
		},
	}
	log.Debugf("subscription request: %v", sub)

	return sub, nil
}

func (s *V2E2Session) createE2Subscription(indChan chan indication.Indication, nodeID string, ranFuncDesc *e2sm_kpm_v2.E2SmKpmRanfunctionDescription) error {
	log.Infof("Connecting to ONOS-E2Sub...%s", s.E2SubEndpoint)

	e2SubHost := strings.Split(s.E2SubEndpoint, ":")[0]
	e2SubPort, err := strconv.Atoi(strings.Split(s.E2SubEndpoint, ":")[1])
	if err != nil {
		log.Error("onos-e2sub's port information or endpoint information is wrong.")
		return err
	}
	e2tHost := strings.Split(s.E2TEndpoint, ":")[0]
	e2tPort, err := strconv.Atoi(strings.Split(s.E2TEndpoint, ":")[1])
	if err != nil {
		log.Error("onos-e2t's port information or endpoint information is wrong.")
		return err
	}

	clientConfig := e2client.Config{
		AppID: "onos-kpimon-v2",
		E2TService: e2client.ServiceConfig{
			Host: e2tHost,
			Port: e2tPort,
		},
		SubscriptionService: e2client.ServiceConfig{
			Host: e2SubHost,
			Port: e2SubPort,
		},
	}

	client, err := e2client.NewClient(clientConfig)
	if err != nil {
		log.Error("Can't open E2Client.")
		return err
	}

	ch := make(chan indication.Indication)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	actionDefMap, err := s.createActionDefinition(ranFuncDesc)
	if err != nil {
		return err
	}

	for _, v := range actionDefMap {
		subReq, err := s.createSubscriptionRequestWithActionDefinition(nodeID, v)
		if err != nil {
			log.Error("Can't create SubsdcriptionRequest message")
			return err
		}

		s.E2SubInstance, err = client.Subscribe(ctx, subReq, ch)
		if err != nil {
			log.Error("Can't send SubscriptionRequest message")
			return err
		}
	}

	log.Infof("Start forwarding Indication message to KPIMON controller")
	for {
		select {
		case indMsg := <-ch:
			indChan <- indMsg
		case trigger := <-s.SubDelTrigger:
			if trigger {
				log.Info("Reset indChan to close subscription")
				return nil
			}
		}
	}
}

func (s *V2E2Session) createEventTriggerData() []byte {
	rtPeriod := s.getReportPeriodFromAdmin()
	log.Infof("Received period value: %v, set the period to: %v", s.ReportPeriodMs, rtPeriod)
	e2SmKpmEventTriggerDefinition, err := pdubuilder.CreateE2SmKpmEventTriggerDefinition(rtPeriod)
	if err != nil {
		log.Errorf("Failed to create event trigger definition data: %v", err)
		return []byte{}
	}

	err = e2SmKpmEventTriggerDefinition.Validate()
	if err != nil {
		log.Errorf("Failed to validate the event trigger definition: %v", err)
		return []byte{}
	}

	protoBytes, err := proto.Marshal(e2SmKpmEventTriggerDefinition)
	if err != nil {
		log.Errorf("Failed to marshal event trigger definition %v", err)
		return []byte{}
	}

	return protoBytes
}

func (s *V2E2Session) getReportPeriodFromAdmin() int32 {
	return int32(s.ReportPeriodMs)
}
