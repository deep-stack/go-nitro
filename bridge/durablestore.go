package bridge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/statechannels/go-nitro/types"
	"github.com/tidwall/buntdb"
)

const BRIDGE_DURABLE_STORE_SUB_DIR = "bridge"

type MirrorChannelDetails struct {
	L1ChannelId types.Destination
	IsCreated   bool
}

type DurableStore struct {
	mirrorChannels *buntdb.DB
	folder         string // the folder where the store's data is stored
}

func NewDurableStore(folder string, config buntdb.Config) (*DurableStore, error) {
	dataFolder := filepath.Join(folder, BRIDGE_DURABLE_STORE_SUB_DIR)
	ds := DurableStore{}

	err := os.MkdirAll(dataFolder, os.ModePerm)
	if err != nil {
		return nil, err
	}

	ds.folder = dataFolder

	ds.mirrorChannels, err = ds.openDB("mirror_channels", config)
	if err != nil {
		return nil, err
	}

	return &ds, nil
}

func (ds *DurableStore) openDB(name string, config buntdb.Config) (*buntdb.DB, error) {
	db, err := buntdb.Open(fmt.Sprintf("%s/%s.db", ds.folder, name))
	if err != nil {
		return nil, err
	}
	err = db.SetConfig(config)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (ds *DurableStore) SetMirrorChannelDetails(l2ChannelId types.Destination, mirrorChannelDetails MirrorChannelDetails) error {
	mirrorChannelDetailsJson, err := json.Marshal(mirrorChannelDetails)
	if err != nil {
		return err
	}

	err = ds.mirrorChannels.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(l2ChannelId.String(), string(mirrorChannelDetailsJson), nil)
		return err
	})
	return err
}

func (ds *DurableStore) GetMirrorChannelDetails(l2ChannelId types.Destination) (mirrorChannelDetails MirrorChannelDetails, err error) {
	var mirrorChannelDetailsJson string

	err = ds.mirrorChannels.View(func(tx *buntdb.Tx) error {
		var err error
		mirrorChannelDetailsJson, err = tx.Get(l2ChannelId.String())
		return err
	})
	if err != nil {
		return mirrorChannelDetails, err
	}

	err = json.Unmarshal([]byte(mirrorChannelDetailsJson), &mirrorChannelDetails)
	if err != nil {
		return mirrorChannelDetails, err
	}

	return mirrorChannelDetails, nil
}

func (ds *DurableStore) GetMirrorChannelDetailsByL1Channel(l1ChannelId types.Destination) (l2ChannelId types.Destination, isCreated bool, err error) {
	err = ds.mirrorChannels.View(func(tx *buntdb.Tx) error {
		return tx.Ascend("", func(key, chJSON string) bool {
			var mirrorChannelDetails MirrorChannelDetails
			err := json.Unmarshal([]byte(chJSON), &mirrorChannelDetails)
			if err != nil {
				return true // not found, continue looking
			}

			if mirrorChannelDetails.L1ChannelId == l1ChannelId {
				l2ChannelId = types.Destination(common.HexToHash(key))
				isCreated = mirrorChannelDetails.IsCreated
				return false // we have found the target: break the Range loop
			}

			return true // not found: continue looking
		})
	})
	if err != nil {
		return l2ChannelId, isCreated, err
	}

	return l2ChannelId, isCreated, nil
}

func (ds *DurableStore) Close() error {
	return ds.mirrorChannels.Close()
}
