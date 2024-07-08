package bridge

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tidwall/buntdb"
)

const BRIDGE_DURABLE_STORE_SUB_DIR = "bridge-store"

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

func (ds *DurableStore) Close() error {
	return ds.mirrorChannels.Close()
}
