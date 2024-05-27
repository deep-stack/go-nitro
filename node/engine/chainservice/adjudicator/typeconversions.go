package NitroAdjudicator

import (
	"math/big"

	"github.com/statechannels/go-nitro/channel/state"
	"github.com/statechannels/go-nitro/channel/state/outcome"
	nc "github.com/statechannels/go-nitro/crypto"
)

func ConvertFixedPart(fp state.FixedPart) INitroTypesFixedPart {
	return INitroTypesFixedPart{
		Participants:      fp.Participants,
		ChannelNonce:      fp.ChannelNonce,
		AppDefinition:     fp.AppDefinition,
		ChallengeDuration: new(big.Int).SetUint64(uint64(fp.ChallengeDuration)),
	}
}

func ConvertVariablePart(vp state.VariablePart) INitroTypesVariablePart {
	return INitroTypesVariablePart{
		AppData: vp.AppData,
		TurnNum: big.NewInt(int64(vp.TurnNum)),
		IsFinal: vp.IsFinal,
		Outcome: convertOutcome(vp.Outcome),
	}
}

func convertOutcome(o outcome.Exit) []ExitFormatSingleAssetExit {
	e := make([]ExitFormatSingleAssetExit, len(o))
	for i, sae := range o {
		e[i].Asset = sae.Asset
		e[i].AssetMetadata = convertAssetMetadata(sae.AssetMetadata)
		e[i].Allocations = convertAllocations(sae.Allocations)
	}
	return e
}

func convertAssetMetadata(am outcome.AssetMetadata) ExitFormatAssetMetadata {
	return ExitFormatAssetMetadata{
		AssetType: uint8(am.AssetType),
		Metadata:  am.Metadata,
	}
}

func convertAllocations(as outcome.Allocations) []ExitFormatAllocation {
	b := make([]ExitFormatAllocation, len(as))
	for i, a := range as {
		b[i].Destination = a.Destination
		b[i].Amount = a.Amount
		b[i].AllocationType = uint8(a.AllocationType)
		b[i].Metadata = a.Metadata
	}
	return b
}

func ConvertSignature(s nc.Signature) INitroTypesSignature {
	sig := INitroTypesSignature{
		V: s.V,
	}
	copy(sig.R[:], s.R)
	copy(sig.S[:], s.S) // TODO we should just use 32 byte types, which would remove the need for this func
	return sig
}

func ConvertSignedStateToFixedPartAndSignedVariablePart(s state.SignedState) (INitroTypesFixedPart, INitroTypesSignedVariablePart) {
	fp := ConvertFixedPart(s.State().FixedPart())
	svp := INitroTypesSignedVariablePart{
		VariablePart: ConvertVariablePart(s.State().VariablePart()),
		Sigs:         make([]INitroTypesSignature, 0, len(s.Signatures())),
	}
	for _, sig := range s.Signatures() {
		if (sig.Equal(nc.Signature{})) {
			continue
		}
		svp.Sigs = append(svp.Sigs, ConvertSignature(sig))
	}

	return fp, svp
}

func ConvertSignedStatesToProof(ss []state.SignedState) []INitroTypesSignedVariablePart {
	svps := make([]INitroTypesSignedVariablePart, 0, len(ss))
	for _, s := range ss {
		_, svp := ConvertSignedStateToFixedPartAndSignedVariablePart(s)
		svps = append(svps, svp)
	}
	return svps
}
