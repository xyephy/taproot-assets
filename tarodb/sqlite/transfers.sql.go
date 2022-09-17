// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.14.0
// source: transfers.sql

package sqlite

import (
	"context"
	"database/sql"
	"time"
)

const applySpendDelta = `-- name: ApplySpendDelta :one
WITH old_script_key_id AS (
    SELECT key_id
    FROM internal_keys
    WHERE raw_key = $3
)
UPDATE assets
SET amount = $1, script_key_id = $2
WHERE script_key_id in (SELECT key_id FROM old_script_key_id)
RETURNING asset_id
`

type ApplySpendDeltaParams struct {
	NewAmount      int64
	NewScriptKeyID int32
	OldScriptKey   []byte
}

func (q *Queries) ApplySpendDelta(ctx context.Context, arg ApplySpendDeltaParams) (int32, error) {
	row := q.db.QueryRowContext(ctx, applySpendDelta, arg.NewAmount, arg.NewScriptKeyID, arg.OldScriptKey)
	var asset_id int32
	err := row.Scan(&asset_id)
	return asset_id, err
}

const deleteAssetWitnesses = `-- name: DeleteAssetWitnesses :exec
DELETE FROM asset_witnesses
WHERE asset_id = ?
`

func (q *Queries) DeleteAssetWitnesses(ctx context.Context, assetID int32) error {
	_, err := q.db.ExecContext(ctx, deleteAssetWitnesses, assetID)
	return err
}

const fetchAssetDeltas = `-- name: FetchAssetDeltas :many
SELECT  
    deltas.old_script_key, deltas.new_amt, deltas.new_script_key, 
    deltas.serialized_witnesses
FROM asset_deltas deltas
JOIN internal_keys new_keys
    ON deltas.new_script_key = new_keys.key_id
WHERE transfer_id = ?
`

type FetchAssetDeltasRow struct {
	OldScriptKey        []byte
	NewAmt              int64
	NewScriptKey        int32
	SerializedWitnesses []byte
}

func (q *Queries) FetchAssetDeltas(ctx context.Context, transferID int32) ([]FetchAssetDeltasRow, error) {
	rows, err := q.db.QueryContext(ctx, fetchAssetDeltas, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []FetchAssetDeltasRow
	for rows.Next() {
		var i FetchAssetDeltasRow
		if err := rows.Scan(
			&i.OldScriptKey,
			&i.NewAmt,
			&i.NewScriptKey,
			&i.SerializedWitnesses,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const insertAssetDelta = `-- name: InsertAssetDelta :exec
INSERT INTO asset_deltas (
    old_script_key, new_amt, new_script_key, serialized_witnesses, transfer_id
) VALUES (
    ?, ?, ?, ?, ?
)
`

type InsertAssetDeltaParams struct {
	OldScriptKey        []byte
	NewAmt              int64
	NewScriptKey        int32
	SerializedWitnesses []byte
	TransferID          int32
}

func (q *Queries) InsertAssetDelta(ctx context.Context, arg InsertAssetDeltaParams) error {
	_, err := q.db.ExecContext(ctx, insertAssetDelta,
		arg.OldScriptKey,
		arg.NewAmt,
		arg.NewScriptKey,
		arg.SerializedWitnesses,
		arg.TransferID,
	)
	return err
}

const insertAssetTransfer = `-- name: InsertAssetTransfer :one
INSERT INTO asset_transfers (
    old_anchor_point, new_anchor_point, new_internal_key, taro_root,
    tapscript_sibling, anchor_tx_id, transfer_time_unix
) VALUES (
    ?, ?, ?, ?, ?, ?, ?
) RETURNING id
`

type InsertAssetTransferParams struct {
	OldAnchorPoint   []byte
	NewAnchorPoint   []byte
	NewInternalKey   int32
	TaroRoot         []byte
	TapscriptSibling []byte
	AnchorTxID       int32
	TransferTimeUnix time.Time
}

func (q *Queries) InsertAssetTransfer(ctx context.Context, arg InsertAssetTransferParams) (int32, error) {
	row := q.db.QueryRowContext(ctx, insertAssetTransfer,
		arg.OldAnchorPoint,
		arg.NewAnchorPoint,
		arg.NewInternalKey,
		arg.TaroRoot,
		arg.TapscriptSibling,
		arg.AnchorTxID,
		arg.TransferTimeUnix,
	)
	var id int32
	err := row.Scan(&id)
	return id, err
}

const queryAssetTransfers = `-- name: QueryAssetTransfers :many
SELECT 
    asset_transfers.old_anchor_point, asset_transfers.new_anchor_point, 
    asset_transfers.taro_root, asset_transfers.tapscript_sibling,
    txns.raw_tx AS anchor_tx_bytes, txns.txid AS anchor_txid,
    txns.txn_id AS anchor_tx_primary_key, transfer_time_unix, 
    keys.raw_key AS internal_key_bytes, keys.key_family AS internal_key_fam,
    keys.key_index AS internal_key_index, id AS transfer_id
FROM asset_transfers
JOIN internal_keys keys
    ON asset_transfers.new_internal_key = keys.key_id
JOIN chain_txns txns
    ON asset_transfers.anchor_tx_id = txns.txn_id
WHERE (
    -- We'll use this clause to filter out for only transfers that are
    -- unconfirmed. But only if the unconf_only field is set.
    -- TODO(roasbeef): just do the confirmed bit, 
    (($1 == 0 OR $1 IS NULL)
        OR
    (($1 == 1) == (length(hex(txns.block_hash)) == 0)))

    AND
    
    -- Here we have another optional query clause to select a given transfer
    -- based on the new_anchor_point, but only if it's specified.
    (length(hex($2)) == 0 OR 
        asset_transfers.new_anchor_point = $2)
)
`

type QueryAssetTransfersParams struct {
	UnconfOnly     interface{}
	NewAnchorPoint interface{}
}

type QueryAssetTransfersRow struct {
	OldAnchorPoint     []byte
	NewAnchorPoint     []byte
	TaroRoot           []byte
	TapscriptSibling   []byte
	AnchorTxBytes      []byte
	AnchorTxid         []byte
	AnchorTxPrimaryKey int32
	TransferTimeUnix   time.Time
	InternalKeyBytes   []byte
	InternalKeyFam     int32
	InternalKeyIndex   int32
	TransferID         int32
}

func (q *Queries) QueryAssetTransfers(ctx context.Context, arg QueryAssetTransfersParams) ([]QueryAssetTransfersRow, error) {
	rows, err := q.db.QueryContext(ctx, queryAssetTransfers, arg.UnconfOnly, arg.NewAnchorPoint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []QueryAssetTransfersRow
	for rows.Next() {
		var i QueryAssetTransfersRow
		if err := rows.Scan(
			&i.OldAnchorPoint,
			&i.NewAnchorPoint,
			&i.TaroRoot,
			&i.TapscriptSibling,
			&i.AnchorTxBytes,
			&i.AnchorTxid,
			&i.AnchorTxPrimaryKey,
			&i.TransferTimeUnix,
			&i.InternalKeyBytes,
			&i.InternalKeyFam,
			&i.InternalKeyIndex,
			&i.TransferID,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const reanchorAssets = `-- name: ReanchorAssets :exec
WITH assets_to_update AS (
    SELECT asset_id
    FROM assets
    JOIN managed_utxos utxos
        ON assets.anchor_utxo_id = utxos.utxo_id
    WHERE utxos.outpoint = $2
)
UPDATE assets
SET anchor_utxo_id = $1
WHERE asset_id IN (SELECT asset_id FROM assets_to_update)
`

type ReanchorAssetsParams struct {
	NewOutpointUtxoID sql.NullInt32
	OldOutpoint       []byte
}

func (q *Queries) ReanchorAssets(ctx context.Context, arg ReanchorAssetsParams) error {
	_, err := q.db.ExecContext(ctx, reanchorAssets, arg.NewOutpointUtxoID, arg.OldOutpoint)
	return err
}