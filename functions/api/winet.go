package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// commandStore implements server.WinetService by queuing inverter commands into
// the pending_commands table. The local service polls this table and executes
// the commands against the physical inverter.
type commandStore struct {
	db       *sql.DB
	deviceID string
}

func newCommandStore(db *sql.DB, deviceID string) *commandStore {
	return &commandStore{db: db, deviceID: deviceID}
}

const insertPendingCommand = `
INSERT INTO pending_commands (device_id, command_type, payload)
VALUES (@p1, @p2, @p3)`

func (c *commandStore) insertCommand(commandType string, payload any) (bool, error) {
	p, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("marshal command payload: %w", err)
	}
	_, err = c.db.Exec(insertPendingCommand,
		sql.Named("p1", c.deviceID),
		sql.Named("p2", commandType),
		sql.Named("p3", string(p)),
	)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (c *commandStore) SendSelfConsumptionCommand() (bool, error) {
	return c.insertCommand("self_consumption", struct{}{})
}

func (c *commandStore) SendBatteryStopCommand() (bool, error) {
	return c.insertCommand("battery_stop", struct{}{})
}

func (c *commandStore) SendDischargeCommand(dischargePower string) (bool, error) {
	return c.insertCommand("discharge", struct {
		DischargePower string `json:"discharge_power"`
	}{DischargePower: dischargePower})
}

func (c *commandStore) SendChargeCommand(chargePower string) (bool, error) {
	return c.insertCommand("charge", struct {
		ChargePower string `json:"charge_power"`
	}{ChargePower: chargePower})
}

func (c *commandStore) SendInverterStateChangeCommand(disable bool) (bool, error) {
	return c.insertCommand("inverter_state_change", struct {
		Disable bool `json:"disable"`
	}{Disable: disable})
}

func (c *commandStore) SetFeedInLimitation(limited bool) (bool, error) {
	return c.insertCommand("set_feed_in_limitation", struct {
		Limited bool `json:"limited"`
	}{Limited: limited})
}
