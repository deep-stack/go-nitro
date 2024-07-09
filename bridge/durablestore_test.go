package bridge_test

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/google/go-cmp/cmp"
	"github.com/statechannels/go-nitro/bridge"
	"github.com/statechannels/go-nitro/internal/testhelpers"
	"github.com/statechannels/go-nitro/types"
	"github.com/tidwall/buntdb"
)

func TestSetGetMirrorChannels(t *testing.T) {
	dataFolder, cleanup := testhelpers.GenerateTempStoreFolder()
	defer cleanup()
	durableStore, err := bridge.NewDurableStore(dataFolder, buntdb.Config{})
	if err != nil {
		t.Fatal(err)
	}

	l1ChannelId := types.Destination(common.HexToHash("0x59846efc80336b0961e4f84a0d974967bfeb04a9b17d90b4610b1a968f00efcd"))
	l2ChannelId := types.Destination(common.HexToHash("0x53494de9354193e864d545e7edfb3915e8b1e210fe4b57585055dff17b2abd30"))
	mirrorChannelDetail := bridge.MirrorChannelDetails{L1ChannelId: l1ChannelId, IsCreated: true}

	err = durableStore.SetMirrorChannelDetails(l2ChannelId, mirrorChannelDetail)
	if err != nil {
		t.Fatal(err)
	}

	got, err := durableStore.GetMirrorChannelDetails(l2ChannelId)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(got, mirrorChannelDetail); diff != "" {
		t.Fatalf("fetched result different than expected %s", diff)
	}

	testL2ChannelId, testIscreated, err := durableStore.GetMirrorChannelDetailsByL1Channel(l1ChannelId)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(l2ChannelId, testL2ChannelId); diff != "" {
		t.Fatalf("fetched result different than expected %s", diff)
	}

	if diff := cmp.Diff(testIscreated, mirrorChannelDetail.IsCreated); diff != "" {
		t.Fatalf("fetched result different than expected %s", diff)
	}
}
