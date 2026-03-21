package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	servermocks "github.com/anicoll/winet-integration/mocks/server"
	api "github.com/anicoll/winet-integration/pkg/server"
)

// helpers

func postJSON(t *testing.T, body any) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func propSeq(props []dbpkg.Property) iter.Seq[dbpkg.Property] {
	return func(yield func(dbpkg.Property) bool) {
		for _, p := range props {
			if !yield(p) {
				return
			}
		}
	}
}

// --- PostBatteryState ---

func TestPostBatteryState_SelfConsumption(t *testing.T) {
	w := servermocks.NewWinetService(t)
	w.EXPECT().SendSelfConsumptionCommand().Return(true, nil)
	svc := New(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostBatteryState(rec, postJSON(t, api.ChangeBatteryStatePayload{
		State: api.SelfConsumption,
	}), "self_consumption")

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPostBatteryState_Stop(t *testing.T) {
	w := servermocks.NewWinetService(t)
	w.EXPECT().SendBatteryStopCommand().Return(true, nil)
	svc := New(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostBatteryState(rec, postJSON(t, api.ChangeBatteryStatePayload{
		State: api.Stop,
	}), "stop")

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPostBatteryState_Charge_SendsPower(t *testing.T) {
	w := servermocks.NewWinetService(t)
	power := "6.6"
	w.EXPECT().SendChargeCommand("6.6").Return(true, nil)
	svc := New(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostBatteryState(rec, postJSON(t, api.ChangeBatteryStatePayload{
		State: api.Charge,
		Power: &power,
	}), "charge")

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPostBatteryState_Discharge_SendsPower(t *testing.T) {
	w := servermocks.NewWinetService(t)
	power := "3.3"
	w.EXPECT().SendDischargeCommand("3.3").Return(true, nil)
	svc := New(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostBatteryState(rec, postJSON(t, api.ChangeBatteryStatePayload{
		State: api.Discharge,
		Power: &power,
	}), "discharge")

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPostBatteryState_Charge_MissingPower_ReturnsError(t *testing.T) {
	w := servermocks.NewWinetService(t)
	svc := New(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostBatteryState(rec, postJSON(t, api.ChangeBatteryStatePayload{
		State: api.Charge,
		// Power is nil — should fail
	}), "charge")

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- PostInverterFeedin ---

func TestPostInverterFeedin_Disable(t *testing.T) {
	w := servermocks.NewWinetService(t)
	w.EXPECT().SetFeedInLimitation(true).Return(true, nil)
	svc := New(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostInverterFeedin(rec, postJSON(t, api.ChangeFeedinPayload{Disable: true}))

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPostInverterFeedin_Enable(t *testing.T) {
	w := servermocks.NewWinetService(t)
	w.EXPECT().SetFeedInLimitation(false).Return(true, nil)
	svc := New(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostInverterFeedin(rec, postJSON(t, api.ChangeFeedinPayload{Disable: false}))

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// --- PostInverterState ---

func TestPostInverterState_Off(t *testing.T) {
	w := servermocks.NewWinetService(t)
	w.EXPECT().SendInverterStateChangeCommand(true).Return(true, nil)
	svc := New(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostInverterState(rec, httptest.NewRequest(http.MethodPost, "/", nil), string(api.Off))

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPostInverterState_On(t *testing.T) {
	w := servermocks.NewWinetService(t)
	w.EXPECT().SendInverterStateChangeCommand(false).Return(true, nil)
	svc := New(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostInverterState(rec, httptest.NewRequest(http.MethodPost, "/", nil), string(api.On))

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// --- GetProperties ---

func TestGetProperties_ReturnsJSON(t *testing.T) {
	props := []dbpkg.Property{
		{ID: 1, Identifier: "XH3000_SN001", Slug: "battery_power", Value: "5.5", UnitOfMeasurement: "kW"},
		{ID: 2, Identifier: "XH3000_SN001", Slug: "battery_soc", Value: "80", UnitOfMeasurement: "%"},
	}
	db := servermocks.NewDatabase(t)
	db.EXPECT().GetLatestProperties(mock.Anything).Return(propSeq(props), nil)
	svc := New(servermocks.NewWinetService(t), db)

	rec := httptest.NewRecorder()
	svc.GetProperties(rec, httptest.NewRequest(http.MethodGet, "/properties", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got []dbpkg.Property
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	assert.Len(t, got, 2)
}

func TestGetProperties_DBError_Returns500(t *testing.T) {
	db := servermocks.NewDatabase(t)
	db.EXPECT().GetLatestProperties(mock.Anything).Return(nil, errors.New("database unavailable"))
	svc := New(servermocks.NewWinetService(t), db)

	rec := httptest.NewRecorder()
	svc.GetProperties(rec, httptest.NewRequest(http.MethodGet, "/properties", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// Ensure the mocks satisfy the interfaces at compile time.
var _ WinetService = (*servermocks.WinetService)(nil)
var _ Database = (*servermocks.Database)(nil)
