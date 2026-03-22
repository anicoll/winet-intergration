package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"iter"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/anicoll/winet-integration/internal/pkg/auth"
	dbpkg "github.com/anicoll/winet-integration/internal/pkg/database/db"
	authmocks "github.com/anicoll/winet-integration/mocks/auth"
	servermocks "github.com/anicoll/winet-integration/mocks/server"
	api "github.com/anicoll/winet-integration/pkg/server"
)

// helpers

// newTestServer builds a server with a nil auth service.
// Safe for tests that don't exercise auth endpoints.
func newTestServer(w WinetService, db Database) *server {
	return New(w, db, nil, false)
}

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
	svc := newTestServer(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostBatteryState(rec, postJSON(t, api.ChangeBatteryStatePayload{
		State: api.SelfConsumption,
	}), "self_consumption")

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPostBatteryState_Stop(t *testing.T) {
	w := servermocks.NewWinetService(t)
	w.EXPECT().SendBatteryStopCommand().Return(true, nil)
	svc := newTestServer(w, servermocks.NewDatabase(t))

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
	svc := newTestServer(w, servermocks.NewDatabase(t))

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
	svc := newTestServer(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostBatteryState(rec, postJSON(t, api.ChangeBatteryStatePayload{
		State: api.Discharge,
		Power: &power,
	}), "discharge")

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPostBatteryState_Charge_MissingPower_ReturnsError(t *testing.T) {
	w := servermocks.NewWinetService(t)
	svc := newTestServer(w, servermocks.NewDatabase(t))

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
	svc := newTestServer(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostInverterFeedin(rec, postJSON(t, api.ChangeFeedinPayload{Disable: true}))

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPostInverterFeedin_Enable(t *testing.T) {
	w := servermocks.NewWinetService(t)
	w.EXPECT().SetFeedInLimitation(false).Return(true, nil)
	svc := newTestServer(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostInverterFeedin(rec, postJSON(t, api.ChangeFeedinPayload{Disable: false}))

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// --- PostInverterState ---

func TestPostInverterState_Off(t *testing.T) {
	w := servermocks.NewWinetService(t)
	w.EXPECT().SendInverterStateChangeCommand(true).Return(true, nil)
	svc := newTestServer(w, servermocks.NewDatabase(t))

	rec := httptest.NewRecorder()
	svc.PostInverterState(rec, httptest.NewRequest(http.MethodPost, "/", nil), string(api.Off))

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestPostInverterState_On(t *testing.T) {
	w := servermocks.NewWinetService(t)
	w.EXPECT().SendInverterStateChangeCommand(false).Return(true, nil)
	svc := newTestServer(w, servermocks.NewDatabase(t))

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
	svc := newTestServer(servermocks.NewWinetService(t), db)

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
	svc := newTestServer(servermocks.NewWinetService(t), db)

	rec := httptest.NewRecorder()
	svc.GetProperties(rec, httptest.NewRequest(http.MethodGet, "/properties", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// --- Auth handler helpers ---

const (
	authTestSecret   = "auth-handler-test-secret-long-enough"
	authTestPassword = "handler-test-password-123"
	authTestUsername = "testuser"
)

func newAuthService(t *testing.T) *auth.Service {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(authTestPassword), bcrypt.MinCost)
	require.NoError(t, err)

	us := authmocks.NewUserStore(t)
	us.EXPECT().GetUserByUsername(mock.Anything, mock.Anything).Return(dbpkg.User{
		ID:           1,
		Username:     authTestUsername,
		PasswordHash: string(h),
	}, nil).Maybe()

	ts := authmocks.NewTokenStore(t)
	ts.EXPECT().StoreRefreshToken(mock.Anything, mock.Anything).Return(nil).Maybe()
	ts.EXPECT().GetRefreshToken(mock.Anything, mock.Anything).Return(dbpkg.RefreshToken{}, errors.New("not found")).Maybe()
	ts.EXPECT().DeleteRefreshToken(mock.Anything, mock.Anything).Return(nil).Maybe()
	ts.EXPECT().DeleteExpiredRefreshTokens(mock.Anything).Return(nil).Maybe()

	return auth.NewService(authTestSecret, 15*time.Minute, 24*time.Hour, us, ts)
}

func newAuthTestServer(t *testing.T) *server {
	t.Helper()
	return New(servermocks.NewWinetService(t), servermocks.NewDatabase(t), newAuthService(t), false)
}

// --- PostAuthLogin ---

func TestPostAuthLogin_Success(t *testing.T) {
	svc := newAuthTestServer(t)

	body, _ := json.Marshal(api.LoginRequest{Username: authTestUsername, Password: authTestPassword})
	rec := httptest.NewRecorder()
	svc.PostAuthLogin(rec, httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body)))

	require.Equal(t, http.StatusOK, rec.Code)

	var resp api.LoginResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.NotEmpty(t, resp.AccessToken)

	// Refresh cookie must be set.
	cookies := rec.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == refreshCookieName {
			found = true
			assert.True(t, c.HttpOnly)
			assert.Equal(t, "/auth/refresh", c.Path)
		}
	}
	assert.True(t, found, "refresh_token cookie should be set")
}

func TestPostAuthLogin_InvalidCredentials_Returns401(t *testing.T) {
	svc := newAuthTestServer(t)

	body, _ := json.Marshal(api.LoginRequest{Username: authTestUsername, Password: "wrong-password-xyz"})
	rec := httptest.NewRecorder()
	svc.PostAuthLogin(rec, httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body)))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPostAuthLogin_BadJSON_Returns400(t *testing.T) {
	svc := newAuthTestServer(t)

	rec := httptest.NewRecorder()
	svc.PostAuthLogin(rec, httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader([]byte("not-json"))))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// --- PostAuthRefresh ---

func TestPostAuthRefresh_Success(t *testing.T) {
	svc := newAuthTestServer(t)

	// Login first to get a refresh cookie.
	body, _ := json.Marshal(api.LoginRequest{Username: authTestUsername, Password: authTestPassword})
	loginRec := httptest.NewRecorder()
	svc.PostAuthLogin(loginRec, httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, loginRec.Code)

	var refreshCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == refreshCookieName {
			refreshCookie = c
		}
	}
	require.NotNil(t, refreshCookie)

	// Use the cookie to refresh.
	refreshReq := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	refreshReq.AddCookie(refreshCookie)
	refreshRec := httptest.NewRecorder()
	svc.PostAuthRefresh(refreshRec, refreshReq)

	require.Equal(t, http.StatusOK, refreshRec.Code)
	var resp api.LoginResponse
	require.NoError(t, json.NewDecoder(refreshRec.Body).Decode(&resp))
	assert.NotEmpty(t, resp.AccessToken)
}

func TestPostAuthRefresh_NoCookie_Returns401(t *testing.T) {
	svc := newAuthTestServer(t)

	rec := httptest.NewRecorder()
	svc.PostAuthRefresh(rec, httptest.NewRequest(http.MethodPost, "/auth/refresh", nil))

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPostAuthRefresh_InvalidCookieValue_Returns401(t *testing.T) {
	svc := newAuthTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "not-a-real-token"})
	rec := httptest.NewRecorder()

	svc.PostAuthRefresh(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- PostAuthLogout ---

func TestPostAuthLogout_WithCookie_Returns204AndClearsCookie(t *testing.T) {
	svc := newAuthTestServer(t)

	// Login to get a refresh token.
	body, _ := json.Marshal(api.LoginRequest{Username: authTestUsername, Password: authTestPassword})
	loginRec := httptest.NewRecorder()
	svc.PostAuthLogin(loginRec, httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, loginRec.Code)

	var refreshCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == refreshCookieName {
			refreshCookie = c
		}
	}
	require.NotNil(t, refreshCookie)

	// Logout.
	logoutReq := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	logoutReq.AddCookie(refreshCookie)
	logoutRec := httptest.NewRecorder()
	svc.PostAuthLogout(logoutRec, logoutReq)

	assert.Equal(t, http.StatusNoContent, logoutRec.Code)

	// Cookie should be expired (MaxAge < 0).
	for _, c := range logoutRec.Result().Cookies() {
		if c.Name == refreshCookieName {
			assert.Less(t, c.MaxAge, 0, "cookie should be cleared")
		}
	}
}

func TestPostAuthLogout_RevokesRefreshToken(t *testing.T) {
	svc := newAuthTestServer(t)

	// Login.
	body, _ := json.Marshal(api.LoginRequest{Username: authTestUsername, Password: authTestPassword})
	loginRec := httptest.NewRecorder()
	svc.PostAuthLogin(loginRec, httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, loginRec.Code)

	var refreshCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == refreshCookieName {
			refreshCookie = c
		}
	}
	require.NotNil(t, refreshCookie)

	// Logout.
	logoutReq := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	logoutReq.AddCookie(refreshCookie)
	svc.PostAuthLogout(httptest.NewRecorder(), logoutReq)

	// Refresh with the same cookie should now fail.
	refreshReq := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	refreshReq.AddCookie(refreshCookie)
	refreshRec := httptest.NewRecorder()
	svc.PostAuthRefresh(refreshRec, refreshReq)

	assert.Equal(t, http.StatusUnauthorized, refreshRec.Code)
}

func TestPostAuthLogout_NoCookie_Returns204(t *testing.T) {
	svc := newAuthTestServer(t)

	rec := httptest.NewRecorder()
	svc.PostAuthLogout(rec, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// --- GetAmberUsageFromTo ---

func TestGetAmberUsageFromTo_ReturnsJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	usage := []dbpkg.Amberusage{
		{
			ID:                1,
			PerKwh:            24.5,
			SpotPerKwh:        6.1,
			StartTime:         now.Add(-30 * time.Minute),
			EndTime:           now,
			Duration:          30,
			ChannelType:       "general",
			ChannelIdentifier: "E1",
			Kwh:               1.234,
			Quality:           "billable",
			Cost:              0.30,
		},
	}
	db := servermocks.NewDatabase(t)
	db.EXPECT().GetAmberUsage(mock.Anything, mock.Anything, mock.Anything).Return(usage, nil)
	svc := newTestServer(servermocks.NewWinetService(t), db)

	from := now.Add(-time.Hour)
	to := now
	rec := httptest.NewRecorder()
	svc.GetAmberUsageFromTo(rec, httptest.NewRequest(http.MethodGet, "/amber/usage/from/to", nil), from, to, api.GetAmberUsageFromToParams{})

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got []api.AmberUsage
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].Id)
	assert.Equal(t, "E1", got[0].ChannelIdentifier)
	assert.Equal(t, "general", got[0].ChannelType)
	assert.Equal(t, float32(1.234), got[0].Kwh)
	assert.Equal(t, api.Billable, got[0].Quality)
	assert.Equal(t, float32(0.30), got[0].Cost)
}

func TestGetAmberUsageFromTo_EmptyResult_ReturnsEmptyArray(t *testing.T) {
	db := servermocks.NewDatabase(t)
	db.EXPECT().GetAmberUsage(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	svc := newTestServer(servermocks.NewWinetService(t), db)

	now := time.Now().UTC()
	rec := httptest.NewRecorder()
	svc.GetAmberUsageFromTo(rec, httptest.NewRequest(http.MethodGet, "/amber/usage/from/to", nil), now.Add(-time.Hour), now, api.GetAmberUsageFromToParams{})

	require.Equal(t, http.StatusOK, rec.Code)
	var got []api.AmberUsage
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	assert.Empty(t, got)
}

func TestGetAmberUsageFromTo_DBError_Returns500(t *testing.T) {
	db := servermocks.NewDatabase(t)
	db.EXPECT().GetAmberUsage(mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("db connection lost"))
	svc := newTestServer(servermocks.NewWinetService(t), db)

	now := time.Now().UTC()
	rec := httptest.NewRecorder()
	svc.GetAmberUsageFromTo(rec, httptest.NewRequest(http.MethodGet, "/amber/usage/from/to", nil), now.Add(-time.Hour), now, api.GetAmberUsageFromToParams{})

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// Ensure the mocks satisfy the interfaces at compile time.
var _ WinetService = (*servermocks.WinetService)(nil)
var _ Database = (*servermocks.Database)(nil)
