package proof

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/lightninglabs/taproot-assets/asset"
	"github.com/lightninglabs/taproot-assets/commitment"
	"github.com/lightninglabs/taproot-assets/fn"
	"github.com/lightninglabs/taproot-assets/internal/test"
	"github.com/lightningnetwork/lnd/build"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

var (
	// proofFileHexFileName is the name of the file that contains the hex
	// proof file data. The proof file contains some proofs generated from
	// an integration test run. See the "gen-itest-test-vectors" make goal.
	proofFileHexFileName = filepath.Join(
		testDataFileName, RegtestProofFileName,
	)

	// proofHexFileName is the name of the file that contains the hex proof
	// data. The proof is generated by an integration test run. See the
	// "gen-itest-test-vectors" make goal.
	proofHexFileName = filepath.Join(testDataFileName, RegtestProofName)

	// ownershipProofHexFileName is the name of the file that contains the
	// hex proof data. The proof is generated by an integration test run.
	// See the "gen-itest-test-vectors" make goal.
	ownershipProofHexFileName = filepath.Join(
		testDataFileName, RegtestOwnershipProofName,
	)

	generatedTestVectorName = "proof_tlv_encoding_generated.json"

	allTestVectorFiles = []string{
		generatedTestVectorName,
		RegtestTestVectorName,
		"proof_tlv_encoding_error_cases.json",
	}
)

func assertEqualCommitmentProof(t *testing.T, expected, actual *CommitmentProof) {
	require.Equal(t, expected.Proof.AssetProof, actual.Proof.AssetProof)
	require.Equal(
		t, expected.Proof.TaprootAssetProof,
		actual.Proof.TaprootAssetProof,
	)
	require.Equal(t, expected.TapSiblingPreimage, actual.TapSiblingPreimage)
}

func assertEqualTaprootProof(t *testing.T, expected, actual *TaprootProof) {
	t.Helper()
	require.Equal(t, expected.OutputIndex, actual.OutputIndex)
	require.Equal(t, expected.InternalKey, actual.InternalKey)
	if expected.CommitmentProof == nil {
		require.Nil(t, actual.CommitmentProof)
	} else {
		assertEqualCommitmentProof(
			t, expected.CommitmentProof, actual.CommitmentProof,
		)
	}
	if expected.TapscriptProof == nil {
		require.Nil(t, actual.TapscriptProof)
	} else {
		require.Equal(t, expected.TapscriptProof, actual.TapscriptProof)
	}
}

func assertEqualProof(t *testing.T, expected, actual *Proof) {
	t.Helper()

	require.Equal(t, expected.PrevOut, actual.PrevOut)
	require.Equal(t, expected.BlockHeader, actual.BlockHeader)
	require.Equal(t, expected.BlockHeight, actual.BlockHeight)
	require.Equal(t, expected.AnchorTx, actual.AnchorTx)
	require.Equal(t, expected.TxMerkleProof, actual.TxMerkleProof)
	require.Equal(t, expected.Asset, actual.Asset)

	assertEqualTaprootProof(t, &expected.InclusionProof, &actual.InclusionProof)

	for i := range expected.ExclusionProofs {
		assertEqualTaprootProof(
			t, &expected.ExclusionProofs[i], &actual.ExclusionProofs[i],
		)
	}

	require.Equal(t, expected.ExclusionProofs, actual.ExclusionProofs)

	if expected.SplitRootProof != nil {
		assertEqualTaprootProof(
			t, expected.SplitRootProof, actual.SplitRootProof,
		)
	} else {
		require.Nil(t, actual.SplitRootProof)
	}

	require.Equal(t, expected.MetaReveal, actual.MetaReveal)

	for i := range expected.AdditionalInputs {
		require.Equal(
			t, expected.AdditionalInputs[i].Version,
			actual.AdditionalInputs[i].Version,
		)
		require.Len(
			t, actual.AdditionalInputs,
			len(expected.AdditionalInputs),
		)
		for j := range expected.AdditionalInputs[i].proofs {
			e, err := expected.AdditionalInputs[i].ProofAt(uint32(j))
			require.NoError(t, err)

			a, err := actual.AdditionalInputs[i].ProofAt(uint32(j))
			require.NoError(t, err)
			assertEqualProof(t, e, a)
		}
	}

	require.Equal(t, expected.ChallengeWitness, actual.ChallengeWitness)
}

func TestProofEncoding(t *testing.T) {
	t.Parallel()

	testBlocks := readTestData(t)
	oddTxBlock := testBlocks[0]

	genesis := asset.RandGenesis(t, asset.Collectible)
	scriptKey := test.RandPubKey(t)
	proof := RandProof(t, genesis, scriptKey, oddTxBlock, 0, 1)

	file, err := NewFile(V0, proof, proof)
	require.NoError(t, err)
	proof.AdditionalInputs = []File{*file, *file}

	var proofBuf bytes.Buffer
	require.NoError(t, proof.Encode(&proofBuf))
	proofBytes := proofBuf.Bytes()

	var decodedProof Proof
	require.NoError(t, decodedProof.Decode(bytes.NewReader(proofBytes)))

	assertEqualProof(t, &proof, &decodedProof)

	// Make sure the proof and proof file prefixes are checked correctly.
	var fileBuf bytes.Buffer
	require.NoError(t, file.Encode(&fileBuf))
	fileBytes := fileBuf.Bytes()

	p := &Proof{}
	err = p.Decode(bytes.NewReader(fileBytes))
	require.ErrorContains(
		t, err, "invalid prefix magic bytes, expected TAPP",
	)

	f := &File{}
	err = f.Decode(bytes.NewReader(proofBytes))
	require.ErrorContains(
		t, err, "invalid prefix magic bytes, expected TAPF",
	)

	require.True(t, IsSingleProof(proofBytes))
	require.True(t, IsProofFile(fileBytes))
	require.False(t, IsProofFile(proofBytes))
	require.False(t, IsSingleProof(fileBytes))
	require.False(t, IsProofFile(nil))
	require.False(t, IsSingleProof(nil))

	// Test with a nil tapscript root in the group reveal.
	proof.GroupKeyReveal.SetTapscriptRoot(nil)
	file, err = NewFile(V0, proof, proof)
	require.NoError(t, err)
	proof.AdditionalInputs = []File{*file, *file}

	proofBuf.Reset()
	require.NoError(t, proof.Encode(&proofBuf))
	var decodedProof2 Proof
	require.NoError(t, decodedProof2.Decode(&proofBuf))

	assertEqualProof(t, &proof, &decodedProof2)

	// Ensure that operations on a proof of unknown version fail.
	unknownFile, err := NewFile(Version(212), proof, proof)
	require.NoError(t, err)

	firstProof, err := unknownFile.ProofAt(0)
	require.Nil(t, firstProof)
	require.ErrorIs(t, err, ErrUnknownVersion)

	firstProofBytes, err := unknownFile.RawProofAt(0)
	require.Nil(t, firstProofBytes)
	require.ErrorIs(t, err, ErrUnknownVersion)

	lastProof, err := unknownFile.LastProof()
	require.Nil(t, lastProof)
	require.ErrorIs(t, err, ErrUnknownVersion)

	lastProofBytes, err := unknownFile.RawLastProof()
	require.Nil(t, lastProofBytes)
	require.ErrorIs(t, err, ErrUnknownVersion)

	err = unknownFile.AppendProof(proof)
	require.ErrorIs(t, err, ErrUnknownVersion)

	err = unknownFile.ReplaceLastProof(proof)
	require.ErrorIs(t, err, ErrUnknownVersion)
}

func genRandomGenesisWithProof(t testing.TB, assetType asset.Type,
	amt *uint64, tapscriptPreimage *commitment.TapscriptPreimage,
	noMetaHash bool, metaReveal *MetaReveal, genesisMutator genMutator,
	genesisRevealMutator genRevealMutator,
	groupRevealMutator groupRevealMutator,
	assetVersion asset.Version) (Proof, *btcec.PrivateKey) {

	t.Helper()

	genesisPrivKey := test.RandPrivKey()
	genesisPubKey := test.PubToKeyDesc(genesisPrivKey.PubKey())

	// If we have a specified meta reveal, then we'll replace the meta hash
	// with the hash of the reveal instead.
	assetGenesis := asset.RandGenesis(t, assetType)
	assetGenesis.OutputIndex = 0
	if metaReveal != nil {
		assetGenesis.MetaHash = metaReveal.MetaHash()
	} else if noMetaHash {
		assetGenesis.MetaHash = [32]byte{}
	}

	if genesisMutator != nil {
		genesisMutator(&assetGenesis)
	}

	groupAmt := uint64(1)
	if amt != nil {
		groupAmt = *amt
	}

	protoAsset := asset.NewAssetNoErr(
		t, assetGenesis, groupAmt, 0, 0,
		asset.NewScriptKeyBip86(genesisPubKey), nil,
		asset.WithAssetVersion(assetVersion),
	)
	assetGroupKey := asset.RandGroupKey(t, assetGenesis, protoAsset)
	groupKeyReveal := asset.NewGroupKeyRevealV0(
		asset.ToSerialized(assetGroupKey.RawKey.PubKey),
		assetGroupKey.TapscriptRoot,
	)

	if groupRevealMutator != nil {
		groupRevealMutator(groupKeyReveal)
	}

	tapCommitment, assets, err := commitment.Mint(
		commitment.RandTapCommitVersion(), assetGenesis, assetGroupKey,
		&commitment.AssetDetails{
			Type:             assetType,
			ScriptKey:        genesisPubKey,
			Amount:           amt,
			Version:          assetVersion,
			LockTime:         0,
			RelativeLockTime: 0,
		},
	)
	require.NoError(t, err)

	// Include 1 or more alt leaves in the anchor output Tap commitment.
	// Since this is also used for generating the test vectors, we don't
	// actually want to have zero alt leaves.
	innerAltLeaves := asset.RandAltLeaves(t, true)

	altCommitment, err := commitment.NewAssetCommitment(innerAltLeaves...)
	require.NoError(t, err)

	err = tapCommitment.Upsert(altCommitment)
	require.NoError(t, err)
	altLeaves := asset.ToAltLeaves(innerAltLeaves)
	genesisAsset := assets[0]
	_, commitmentProof, err := tapCommitment.Proof(
		genesisAsset.TapCommitmentKey(),
		genesisAsset.AssetCommitmentKey(),
	)
	require.NoError(t, err)

	var tapscriptSibling *chainhash.Hash
	if tapscriptPreimage != nil {
		tapscriptSibling, err = tapscriptPreimage.TapHash()
		require.NoError(t, err)
	}

	internalKey := test.SchnorrPubKey(t, genesisPrivKey)
	tapscriptRoot := tapCommitment.TapscriptRoot(tapscriptSibling)
	taprootKey := txscript.ComputeTaprootOutputKey(
		internalKey, tapscriptRoot[:],
	)
	taprootScript := test.ComputeTaprootScript(t, taprootKey)
	genesisTx := &wire.MsgTx{
		Version: 2,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: assetGenesis.FirstPrevOut,
		}},
		TxOut: []*wire.TxOut{{
			PkScript: taprootScript,
			Value:    330,
		}},
	}
	merkleTree := blockchain.BuildMerkleTreeStore(
		[]*btcutil.Tx{btcutil.NewTx(genesisTx)}, false,
	)
	merkleRoot := merkleTree[len(merkleTree)-1]

	// We'll use the genesis hash of the mainnet chain as the parent block.
	blockHeader := wire.NewBlockHeader(
		0, chaincfg.MainNetParams.GenesisHash, merkleRoot, 0, 0,
	)
	blockHeader.Timestamp = time.Unix(test.RandInt[int64](), 0)

	// We'll set the block height to 1, as the genesis block is at height 0.
	blockHeight := uint32(1)

	txMerkleProof, err := NewTxMerkleProof([]*wire.MsgTx{genesisTx}, 0)
	require.NoError(t, err)

	genReveal := &assetGenesis
	if genesisRevealMutator != nil {
		genReveal = genesisRevealMutator(genReveal)
	}

	return Proof{
		PrevOut:       assetGenesis.FirstPrevOut,
		BlockHeader:   *blockHeader,
		BlockHeight:   blockHeight,
		AnchorTx:      *genesisTx,
		TxMerkleProof: *txMerkleProof,
		Asset:         *genesisAsset,
		InclusionProof: TaprootProof{
			OutputIndex: 0,
			InternalKey: internalKey,
			CommitmentProof: &CommitmentProof{
				Proof:              *commitmentProof,
				TapSiblingPreimage: tapscriptPreimage,
			},
			TapscriptProof: nil,
		},
		MetaReveal:       metaReveal,
		ExclusionProofs:  nil,
		AdditionalInputs: nil,
		GenesisReveal:    genReveal,
		GroupKeyReveal:   groupKeyReveal,
		AltLeaves:        altLeaves,
	}, genesisPrivKey
}

type genMutator func(*asset.Genesis)

type groupRevealMutator func(asset.GroupKeyReveal)

type genRevealMutator func(*asset.Genesis) *asset.Genesis

func TestGenesisProofVerification(t *testing.T) {
	t.Parallel()

	// Create a script tree that we'll use for our tapscript sibling test
	// cases.
	scriptInternalKey := test.RandPrivKey().PubKey()
	leaf1 := test.ScriptHashLock(t, []byte("foobar"))
	leaf2 := test.ScriptSchnorrSig(t, scriptInternalKey)
	testLeafPreimage, err := commitment.NewPreimageFromLeaf(leaf1)
	require.NoError(t, err)

	// The order doesn't matter here as they are sorted before hashing.
	branch := txscript.NewTapBranch(leaf1, leaf2)
	testBranchPreimage := commitment.NewPreimageFromBranch(branch)
	amount := uint64(5000)

	testCases := []struct {
		name                 string
		assetType            asset.Type
		amount               *uint64
		assetVersion         asset.Version
		tapscriptPreimage    *commitment.TapscriptPreimage
		metaReveal           *MetaReveal
		noMetaHash           bool
		noGroup              bool
		genesisMutator       genMutator
		genesisRevealMutator genRevealMutator
		groupRevealMutator   groupRevealMutator
		expectedErr          error
	}{
		{
			name:       "collectible genesis",
			assetType:  asset.Collectible,
			noMetaHash: true,
		},
		{
			name:         "collectible genesis v1 asset version",
			assetType:    asset.Collectible,
			noMetaHash:   true,
			assetVersion: asset.V1,
		},
		{
			name:              "collectible with leaf preimage",
			assetType:         asset.Collectible,
			tapscriptPreimage: testLeafPreimage,
			noMetaHash:        true,
		},
		{
			name:              "collectible with branch preimage",
			assetType:         asset.Collectible,
			tapscriptPreimage: &testBranchPreimage,
			noMetaHash:        true,
		},
		{
			name:       "normal genesis",
			assetType:  asset.Normal,
			amount:     &amount,
			noMetaHash: true,
		},
		{
			name:         "normal genesis v1 asset version",
			assetType:    asset.Normal,
			amount:       &amount,
			noMetaHash:   true,
			assetVersion: asset.V1,
		},
		{
			name:              "normal with leaf preimage",
			assetType:         asset.Normal,
			amount:            &amount,
			tapscriptPreimage: testLeafPreimage,
			noMetaHash:        true,
		},
		{
			name:              "normal with branch preimage",
			assetType:         asset.Normal,
			amount:            &amount,
			tapscriptPreimage: &testBranchPreimage,
			noMetaHash:        true,
		},
		{
			name:      "normal asset with a meta reveal",
			assetType: asset.Normal,
			amount:    &amount,
			metaReveal: &MetaReveal{
				Data: []byte("meant in croking nevermore"),
			},
		},
		{
			name:      "collectible with a meta reveal",
			assetType: asset.Collectible,
			metaReveal: &MetaReveal{
				Data: []byte("shall be lifted nevermore"),
			},
		},
		{
			name:      "collectible invalid meta reveal",
			assetType: asset.Collectible,
			metaReveal: &MetaReveal{
				Data: []byte("shall be lifted nevermore"),
			},
			genesisMutator: func(genesis *asset.Genesis) {
				// Modify the genesis to make the meta reveal
				// invalid.
				genesis.MetaHash[0] ^= 1
			},
			expectedErr: ErrGenesisRevealMetaHashMismatch,
		},
		{
			name:        "normal asset has meta hash no meta reveal",
			assetType:   asset.Normal,
			amount:      &amount,
			expectedErr: ErrGenesisRevealMetaRevealRequired,
		},
		{
			name: "collectible asset has meta hash no " +
				"meta reveal",
			assetType:   asset.Collectible,
			expectedErr: ErrGenesisRevealMetaRevealRequired,
		},
		{
			name:       "missing genesis reveal",
			assetType:  asset.Collectible,
			noMetaHash: true,
			genesisRevealMutator: func(
				g *asset.Genesis) *asset.Genesis {

				return nil
			},
			expectedErr: ErrGenesisRevealRequired,
		},
		{
			name:      "meta reveal with unknown odd type",
			assetType: asset.Collectible,
			metaReveal: &MetaReveal{
				Data: []byte("{\"foo\": \"bar\"}"),
				Type: MetaJson,
				UnknownOddTypes: tlv.TypeMap{
					//nolint:lll
					test.TestVectorAllowedUnknownType: []byte(
						"decimalDisplay?",
					),
				},
			},
		},
		{
			name:       "genesis reveal asset ID mismatch",
			assetType:  asset.Normal,
			amount:     &amount,
			noMetaHash: true,
			genesisRevealMutator: func(
				g *asset.Genesis) *asset.Genesis {

				gCopy := *g
				gCopy.Tag += "mismatch"
				return &gCopy
			},
			expectedErr: ErrGenesisRevealAssetIDMismatch,
		},
		{
			name:      "genesis reveal prev out mismatch",
			assetType: asset.Collectible,
			genesisRevealMutator: func(
				g *asset.Genesis) *asset.Genesis {

				gCopy := *g
				gCopy.FirstPrevOut = test.RandOp(t)
				return &gCopy
			},
			expectedErr: ErrGenesisRevealPrevOutMismatch,
		},
		{
			name:       "genesis reveal output index mismatch",
			assetType:  asset.Normal,
			amount:     &amount,
			noMetaHash: true,
			genesisRevealMutator: func(
				g *asset.Genesis) *asset.Genesis {

				gCopy := *g
				gCopy.OutputIndex = uint32(
					test.RandInt[int32](),
				)
				return &gCopy
			},
			expectedErr: ErrGenesisRevealOutputIndexMismatch,
		},
		{
			name:       "group key reveal invalid key",
			assetType:  asset.Collectible,
			noMetaHash: true,
			groupRevealMutator: func(gkr asset.GroupKeyReveal) {
				rawKey := gkr.RawKey()
				rawKey[0] = 0x01
				gkr.SetRawKey(rawKey)
			},
			expectedErr: secp256k1.ErrPubKeyInvalidFormat,
		},
		{
			name:       "group key reveal mismatched tweaked key",
			assetType:  asset.Normal,
			amount:     &amount,
			noMetaHash: true,
			groupRevealMutator: func(gkr asset.GroupKeyReveal) {
				gkr.SetTapscriptRoot(test.RandBytes(32))
			},
			expectedErr: ErrGroupKeyRevealMismatch,
		},
	}

	testVectors := &TestVectors{}
	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(tt *testing.T) {
			genesisProof, _ := genRandomGenesisWithProof(
				tt, tc.assetType, tc.amount,
				tc.tapscriptPreimage, tc.noMetaHash,
				tc.metaReveal, tc.genesisMutator,
				tc.genesisRevealMutator, tc.groupRevealMutator,
				tc.assetVersion,
			)
			_, err := genesisProof.Verify(
				context.Background(), nil, MockHeaderVerifier,
				MockMerkleVerifier, MockGroupVerifier,
				MockChainLookup,
			)
			require.ErrorIs(t, err, tc.expectedErr)

			var buf bytes.Buffer
			err = genesisProof.Encode(&buf)
			require.NoError(tt, err)

			if tc.expectedErr == nil {
				testVectors.ValidTestCases = append(
					testVectors.ValidTestCases,
					&ValidTestCase{
						Proof: NewTestFromProof(
							t, &genesisProof,
						),
						Expected: hex.EncodeToString(
							buf.Bytes(),
						),
						Comment: tc.name,
					},
				)
			}
		})
	}

	// Write test vectors to file. This is a no-op if the "gen_test_vectors"
	// build tag is not set.
	test.WriteTestVectors(t, generatedTestVectorName, testVectors)
}

// TestProofBlockHeaderVerification ensures that an error returned by the
// HeaderVerifier callback is correctly propagated by the Verify proof method.
func TestProofBlockHeaderVerification(t *testing.T) {
	t.Parallel()

	proof, _ := genRandomGenesisWithProof(
		t, asset.Collectible, nil, nil, true, nil, nil, nil, nil, 0,
	)

	// Create a base reference for the block header and block height. We
	// will later modify these proof fields.
	var (
		originalBlockHeader = proof.BlockHeader
		originalBlockHeight = proof.BlockHeight
	)

	// Header verifier compares given header to expected header. Verifier
	// does not return error.
	errHeaderVerifier := fmt.Errorf("invalid block header")
	headerVerifier := func(header wire.BlockHeader, height uint32) error {
		// Compare given block header against base reference block
		// header.
		if header != originalBlockHeader || height != originalBlockHeight {
			return errHeaderVerifier
		}
		return nil
	}

	// Verify that the original proof block header is as expected and
	// therefore an error is not returned.
	_, err := proof.Verify(
		context.Background(), nil, headerVerifier, MockMerkleVerifier,
		MockGroupVerifier, MockChainLookup,
	)
	require.NoError(t, err)

	// Modify proof block header, then check that the verification function
	// propagates the correct error.
	proof.BlockHeader.Nonce += 1
	_, actualErr := proof.Verify(
		context.Background(), nil, headerVerifier, MockMerkleVerifier,
		MockGroupVerifier, MockChainLookup,
	)
	require.ErrorIs(t, actualErr, errHeaderVerifier)

	// Reset proof block header.
	proof.BlockHeader.Nonce = originalBlockHeader.Nonce

	// Modify proof block height, then check that the verification function
	// propagates the correct error.
	proof.BlockHeight += 1
	_, actualErr = proof.Verify(
		context.Background(), nil, headerVerifier, MockMerkleVerifier,
		MockGroupVerifier, MockChainLookup,
	)
	require.ErrorIs(t, actualErr, errHeaderVerifier)
}

// TestProofFileVerification ensures that the proof file encoding and decoding
// works as expected.
func TestProofFileVerification(t *testing.T) {
	proofHex, err := os.ReadFile(proofFileHexFileName)
	require.NoError(t, err)

	proofBytes, err := hex.DecodeString(
		strings.Trim(string(proofHex), "\n"),
	)
	require.NoError(t, err)

	f := &File{}
	err = f.Decode(bytes.NewReader(proofBytes))
	require.NoError(t, err)

	_, err = f.Verify(
		context.Background(), MockHeaderVerifier, MockMerkleVerifier,
		MockGroupVerifier, MockChainLookup,
	)
	require.NoError(t, err)

	// Ensure that verification of a proof of unknown version fails.
	f.Version = Version(212)

	lastAsset, err := f.Verify(
		context.Background(), MockHeaderVerifier, MockMerkleVerifier,
		MockGroupVerifier, MockChainLookup,
	)
	require.Nil(t, lastAsset)
	require.ErrorIs(t, err, ErrUnknownVersion)
}

// TestProofVerification ensures that the proof encoding and decoding works as
// expected.
func TestProofVerification(t *testing.T) {
	proofHex, err := os.ReadFile(proofHexFileName)
	require.NoError(t, err)

	proofBytes, err := hex.DecodeString(
		strings.Trim(string(proofHex), "\n"),
	)
	require.NoError(t, err)

	p := &Proof{}
	err = p.Decode(bytes.NewReader(proofBytes))
	require.NoError(t, err)

	assetID := p.Asset.ID()
	t.Logf("Proof asset ID: %x", assetID[:])

	t.Logf("Proof anchor TX: %v", spew.Sdump(p.AnchorTx))

	inclusionTxOut := p.AnchorTx.TxOut[p.InclusionProof.OutputIndex]
	t.Logf("Proof inclusion tx out: %x", inclusionTxOut.PkScript)
	proofKeys, err := p.InclusionProof.DeriveByAssetInclusion(&p.Asset, nil)
	require.NoError(t, err)

	t.Logf("Proof internal key: %x",
		p.InclusionProof.InternalKey.SerializeCompressed())

	for proofKey, commit := range proofKeys {
		rootHash := commit.TapscriptRoot(nil)
		t.Logf("Proof root hash: %x", rootHash[:])
		logString := "CommitmentNonV2"
		if commit.Version == commitment.TapCommitmentV2 {
			logString = "CommitmentV2"
		}

		t.Logf("%s proof key: %x", logString, proofKey)
	}

	var buf bytes.Buffer
	require.NoError(t, p.Asset.Encode(&buf))
	t.Logf("Proof asset encoded: %x", buf.Bytes())

	ta := asset.NewTestFromAsset(t, &p.Asset)
	assetJSON, err := json.Marshal(ta)
	require.NoError(t, err)

	t.Logf("Proof asset JSON: %s", assetJSON)

	// If we have a challenge witness, we can verify that without having the
	// previous proof.
	if len(p.ChallengeWitness) > 0 {
		_, err = p.Verify(
			context.Background(), nil, MockHeaderVerifier,
			MockMerkleVerifier, MockGroupVerifier, MockChainLookup,
		)
		require.NoError(t, err)
	}

	// Verifying the inclusion and exclusion proofs can also be done without
	// the previous proof.
	_, err = p.VerifyProofs()
	require.NoError(t, err)

	// Ensure that verification of a proof of unknown version fails.
	p.Version = TransitionVersion(212)

	lastAsset, err := p.Verify(
		context.Background(), nil, MockHeaderVerifier,
		MockMerkleVerifier, MockGroupVerifier, MockChainLookup,
	)
	require.Nil(t, lastAsset)
	require.ErrorIs(t, err, ErrUnknownVersion)
}

// TestOwnershipProofVerification ensures that the ownership proof encoding and
// decoding as well as the verification works as expected.
func TestOwnershipProofVerification(t *testing.T) {
	proofHex, err := os.ReadFile(ownershipProofHexFileName)
	require.NoError(t, err)

	proofBytes, err := hex.DecodeString(
		strings.Trim(string(proofHex), "\n"),
	)
	require.NoError(t, err)

	p := &Proof{}
	err = p.Decode(bytes.NewReader(proofBytes))
	require.NoError(t, err)

	snapshot, err := p.Verify(
		context.Background(), nil, MockHeaderVerifier,
		MockMerkleVerifier, MockGroupVerifier, MockChainLookup,
	)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
}

// TestProofReplacement ensures that proofs can be replaced in a proof file.
func TestProofReplacement(t *testing.T) {
	// We create a file with 1k proofs.
	const numProofs = 1_000
	lotsOfProofs := make([]Proof, numProofs)
	for i := 0; i < numProofs; i++ {
		amt := uint64(i + 1)
		assetVersion := asset.Version(i % 2)
		lotsOfProofs[i], _ = genRandomGenesisWithProof(
			t, asset.Normal, &amt, nil, false, nil, nil, nil, nil,
			assetVersion,
		)
	}

	f, err := NewFile(V0, lotsOfProofs...)
	require.NoError(t, err)

	assertIndex := func(idx uint32, amt uint64) {
		p, fileIndex, err := f.LocateProof(func(proof *Proof) bool {
			return proof.Asset.Amount == amt
		})
		require.NoError(t, err)

		require.Equal(t, idx, fileIndex)
		require.Equal(t, amt, p.Asset.Amount)
	}
	assertIndex(0, 1)
	assertIndex(999, 1000)

	// We'll now go ahead and randomly replace 100 proofs.
	const numReplacements = 100
	for i := 0; i < numReplacements; i++ {
		amt := uint64(1000*numReplacements - i)
		assetVersion := asset.Version(i % 2)

		// We'll generate a random proof, and then replace a random
		// proof in the file with it.
		proof, _ := genRandomGenesisWithProof(
			t, asset.Normal, &amt, nil, false, nil, nil, nil, nil,
			assetVersion,
		)
		idx := test.RandIntn(numProofs)
		err := f.ReplaceProofAt(uint32(idx), proof)
		require.NoError(t, err)

		assertIndex(uint32(idx), amt)
	}

	// We also replace the very first and very last ones (to test the
	// boundary conditions).
	amt := uint64(1337)
	firstProof, _ := genRandomGenesisWithProof(
		t, asset.Normal, &amt, nil, false, nil, nil, nil, nil, asset.V1,
	)
	err = f.ReplaceProofAt(0, firstProof)
	require.NoError(t, err)
	assertIndex(0, 1337)

	amt = uint64(2016)
	lastProof, _ := genRandomGenesisWithProof(
		t, asset.Normal, &amt, nil, false, nil, nil, nil, nil, asset.V0,
	)
	err = f.ReplaceProofAt(uint32(f.NumProofs()-1), lastProof)
	require.NoError(t, err)
	assertIndex(uint32(f.NumProofs()-1), 2016)

	// Make sure we can still properly encode and decode the file.
	var buf bytes.Buffer
	err = f.Encode(&buf)
	require.NoError(t, err)

	f2, err := NewFile(V0)
	require.NoError(t, err)

	err = f2.Decode(&buf)
	require.NoError(t, err)

	require.Len(t, f2.proofs, numProofs)
	require.Equal(t, f2.proofs, f.proofs)
}

func BenchmarkProofEncoding(b *testing.B) {
	amt := uint64(5000)

	// Start with a minted genesis asset.
	genesisProof, _ := genRandomGenesisWithProof(
		b, asset.Normal, &amt, nil, false, nil, nil, nil, nil, asset.V0,
	)

	// We create a file with 10k proofs (the same one) and test encoding/
	// decoding performance.
	const numProofs = 10_000
	lotsOfProofs := make([]Proof, numProofs)
	for i := 0; i < numProofs; i++ {
		lotsOfProofs[i] = genesisProof
	}

	f, err := NewFile(V0, lotsOfProofs...)
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	// Only this part is measured.
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		err = f.Encode(&buf)
		require.NoError(b, err)

		f2, err := NewFile(V0)
		require.NoError(b, err)

		err = f2.Decode(&buf)
		require.NoError(b, err)

		require.Len(b, f2.proofs, numProofs)
	}
}

// TestBIPTestVectors tests that the BIP test vectors are passing.
func TestBIPTestVectors(t *testing.T) {
	t.Parallel()

	for idx := range allTestVectorFiles {
		var (
			fileName    = allTestVectorFiles[idx]
			testVectors = &TestVectors{}
		)
		test.ParseTestVectors(t, fileName, &testVectors)
		t.Run(fileName, func(tt *testing.T) {
			tt.Parallel()

			runBIPTestVector(tt, testVectors)
		})
	}
}

// runBIPTestVector runs the tests in a single BIP test vector file.
func runBIPTestVector(t *testing.T, testVectors *TestVectors) {
	for _, validCase := range testVectors.ValidTestCases {
		validCase := validCase

		t.Run(validCase.Comment, func(tt *testing.T) {
			tt.Parallel()

			p := validCase.Proof.ToProof(tt)

			var buf bytes.Buffer
			err := p.Encode(&buf)
			require.NoError(tt, err)

			areEqual := validCase.Expected == hex.EncodeToString(
				buf.Bytes(),
			)

			// Make sure the proof in the test vectors doesn't use
			// a record type we haven't marked as known/supported
			// yet. If the following check fails, you need to update
			// the KnownProofTypes set.
			for _, record := range p.EncodeRecords() {
				// Test vectors may contain this one type to
				// demonstrate that it is not rejected.
				if record.Type() ==
					test.TestVectorAllowedUnknownType {

					continue
				}

				require.Contains(
					tt, KnownProofTypes, record.Type(),
				)
			}

			checkTaprootProofTypes(tt, p.InclusionProof)
			for i := range p.ExclusionProofs {
				checkTaprootProofTypes(tt, p.ExclusionProofs[i])
			}

			if p.MetaReveal != nil {
				metaRecords := p.MetaReveal.EncodeRecords()
				for _, record := range metaRecords {
					// Test vectors may contain this one
					// type to demonstrate that it is not
					// rejected.
					// nolint:lll
					if record.Type() ==
						test.TestVectorAllowedUnknownType {

						continue
					}

					require.Contains(
						tt, KnownMetaRevealTypes,
						record.Type(),
					)
				}
			}

			// Create nice diff if things don't match.
			if !areEqual {
				expectedProof := &Proof{}
				proofBytes, err := hex.DecodeString(
					strings.Trim(validCase.Expected, "\n"),
				)
				require.NoError(t, err)

				err = expectedProof.Decode(bytes.NewReader(
					proofBytes,
				))
				require.NoError(tt, err)

				require.Equal(tt, expectedProof, p)

				// Make sure we still fail the test.
				require.Equal(
					tt, validCase.Expected,
					hex.EncodeToString(buf.Bytes()),
				)
			}

			// We also want to make sure that the proof is decoded
			// correctly from the encoded TLV stream.
			decoded := &Proof{}
			err = decoded.Decode(hex.NewDecoder(
				strings.NewReader(validCase.Expected),
			))
			require.NoError(tt, err)

			require.Equal(tt, p, decoded)
		})
	}

	for _, invalidCase := range testVectors.ErrorTestCases {
		invalidCase := invalidCase

		t.Run(invalidCase.Comment, func(tt *testing.T) {
			tt.Parallel()

			require.PanicsWithValue(tt, invalidCase.Error, func() {
				invalidCase.Proof.ToProof(tt)
			})
		})
	}
}

// checkTaprootProofTypes ensures that the taproot proof contains only known
// TLV types.
func checkTaprootProofTypes(t *testing.T, p TaprootProof) {
	for _, record := range p.EncodeRecords() {
		// Test vectors may contain this one type to demonstrate that
		// it is not rejected.
		if record.Type() ==
			test.TestVectorAllowedUnknownType {

			continue
		}

		require.Contains(t, KnownTaprootProofTypes, record.Type())
	}

	if p.CommitmentProof != nil {
		for _, record := range p.CommitmentProof.EncodeRecords() {
			// Test vectors may contain this one type to demonstrate
			// that it is not rejected.
			if record.Type() ==
				test.TestVectorAllowedUnknownType {

				continue
			}

			require.Contains(
				t, KnownCommitmentProofTypes, record.Type(),
			)

			tap := p.CommitmentProof.TaprootAssetProof
			types := commitment.KnownTaprootAssetProofTypes
			for _, record := range tap.Records() {
				require.Contains(
					t, types, record.Type(),
				)
			}

			if p.CommitmentProof.AssetProof != nil {
				ap := p.CommitmentProof.AssetProof
				types := commitment.KnownAssetProofTypes
				for _, record := range ap.Records() {
					require.Contains(
						t, types, record.Type(),
					)
				}
			}
		}
	}

	if p.TapscriptProof != nil {
		for _, record := range p.TapscriptProof.EncodeRecords() {
			// Test vectors may contain this one type to demonstrate
			// that it is not rejected.
			if record.Type() ==
				test.TestVectorAllowedUnknownType {

				continue
			}

			require.Contains(
				t, KnownTapscriptProofTypes, record.Type(),
			)
		}
	}
}

// TestProofUnknownOddType tests that an unknown odd type is allowed in a proof
// and that we can still arrive at the correct serialized version with it.
func TestProofUnknownOddType(t *testing.T) {
	t.Parallel()

	testBlocks := readTestData(t)
	oddTxBlock := testBlocks[0]

	genesis := asset.RandGenesis(t, asset.Collectible)
	scriptKey := test.RandPubKey(t)
	knownProof := RandProof(t, genesis, scriptKey, oddTxBlock, 0, 1)

	var knownProofBytes []byte
	test.RunUnknownOddTypeTest(
		t, &knownProof, &asset.ErrUnknownType{},
		func(buf *bytes.Buffer, proof *Proof) error {
			err := proof.Encode(buf)

			knownProofBytes = fn.CopySlice(buf.Bytes())

			return err
		},
		func(buf *bytes.Buffer) (*Proof, error) {
			var parsedProof Proof
			return &parsedProof, parsedProof.Decode(buf)
		},
		func(parsedProof *Proof, unknownTypes tlv.TypeMap) {
			require.Equal(
				t, unknownTypes, parsedProof.UnknownOddTypes,
			)

			// The proof should've changed, to make sure the unknown
			// value was taken into account when creating the
			// serialized proof.
			var newBuf bytes.Buffer
			err := parsedProof.Encode(&newBuf)
			require.NoError(t, err)

			require.NotEqual(t, knownProofBytes, newBuf.Bytes())

			parsedProof.UnknownOddTypes = nil
			require.Equal(t, &knownProof, parsedProof)
		},
	)
}

func init() {
	logWriter := build.NewRotatingLogWriter()
	logger := logWriter.GenSubLogger(Subsystem, func() {})
	logWriter.RegisterSubLogger(Subsystem, logger)
	UseLogger(logger)
}
