package blockchain

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"math/big"
	"time"
)

const (
	// PaymentChannelIdHeader is a MultiPartyEscrow contract payment channel
	// id. Value is a string containing a decimal number.
	PaymentChannelIdHeader = "snet-payment-channel-id"
	// PaymentChannelNonceHeader is a payment channel nonce value. Value is a
	// string containing a decimal number.
	PaymentChannelNonceHeader = "snet-payment-channel-nonce"
	// PaymentChannelAmountHeader is an amount of payment channel value
	// which server is authorized to withdraw after handling the RPC call.
	// Value is a string containing a decimal number.
	PaymentChannelAmountHeader = "snet-payment-channel-amount"
	// PaymentChannelSignatureHeader is a signature of the client to confirm
	// amount withdrawing authorization. Value is an array of bytes.
	PaymentChannelSignatureHeader = "snet-payment-channel-signature-bin"
)

// TODO: add formatters for PaymentChannelKey, PaymentChannelData
// TODO: document public API

type PaymentChannelKey struct {
	Id    *big.Int
	Nonce *big.Int
}

type PaymentChannelState int

const (
	Open   PaymentChannelState = 0
	Closed PaymentChannelState = 1
)

type PaymentChannelData struct {
	State            PaymentChannelState
	Sender           common.Address
	FullAmount       *big.Int
	Expiration       time.Time
	AuthorizedAmount *big.Int
	Signature        []byte
}

type PaymentChannelStorage interface {
	Get(key *PaymentChannelKey) (paymentChannel *PaymentChannelData, err error)
	CompareAndSwap(key *PaymentChannelKey, prevState *PaymentChannelData, newState *PaymentChannelData) (err error)
}

type PaymentData struct {
	Income *big.Int
}

type AmountValidator interface {
	Validate(*PaymentData) (err *status.Status)
}

// escrowPaymentHandler implements paymentHandlerType interface
type escrowPaymentHandler struct {
	md              metadata.MD
	storage         PaymentChannelStorage
	processor       *Processor
	amountValidator AmountValidator
}

func newEscrowPaymentHandler() *escrowPaymentHandler {
	return &escrowPaymentHandler{}
}

type paymentData struct {
	channelKey *PaymentChannelKey
	amount     *big.Int
	signature  []byte
}

func (h *escrowPaymentHandler) validatePayment() error {
	payment, err := h.getPaymentFromMetadata()
	if err != nil {
		return err
	}
	return h.validatePaymentInternal(payment)
}

func (h *escrowPaymentHandler) validatePaymentInternal(payment *paymentData) (err error) {
	var log = log.WithField("payment", payment)

	paymentChannel, err := h.storage.Get(payment.channelKey)
	if err != nil {
		log.WithError(err).Warn("Payment channel not found")
		// TODO: job.go code always returns codes.Unauthenticated when
		// validations fails
		return status.Errorf(codes.FailedPrecondition, "payment channel \"%v\" not found", payment.channelKey)
	}
	log = log.WithField("paymentChannel", paymentChannel)

	if paymentChannel.State != Open {
		log.Warn("Payment channel is not opened")
		return status.Errorf(codes.FailedPrecondition, "payment channel \"%v\" is not opened", payment.channelKey)
	}

	signerAddress, err := h.getSignerAddressFromPayment(payment)
	if err != nil {
		log.WithError(err).Warn("Unable to get public key from payment")
		return status.Errorf(codes.Unauthenticated, "payment signature is not valid")
	}

	if *signerAddress != paymentChannel.Sender {
		log.WithField("signerAddress", signerAddress).Warn("Channel sender is not equal to payment signer")
		return status.Errorf(codes.Unauthenticated, "payment is not signed by channel sender")
	}

	now := time.Now()
	if paymentChannel.Expiration.Before(now) {
		log.WithField("now", now).Warn("Channel is expired")
		return status.Errorf(codes.FailedPrecondition, "payment channel is expired since \"%v\"", paymentChannel.Expiration)
	}

	if paymentChannel.FullAmount.Cmp(payment.amount) < 0 {
		log.Warn("Not enough tokens on payment channel")
		return status.Errorf(codes.FailedPrecondition, "not enough tokens on payment channel, channel amount: %v, payment amount: %v ", paymentChannel.FullAmount, payment.amount)
	}

	income := big.NewInt(0)
	income.Sub(payment.amount, paymentChannel.AuthorizedAmount)
	s := h.amountValidator.Validate(&PaymentData{Income: income})
	if s != nil {
		return s.Err()
	}

	// TODO: current job code comletes payment iff service returned no error
	err = h.storage.CompareAndSwap(
		payment.channelKey,
		paymentChannel,
		&PaymentChannelData{
			State:            paymentChannel.State,
			FullAmount:       paymentChannel.FullAmount,
			Expiration:       paymentChannel.Expiration,
			AuthorizedAmount: payment.amount,
			Signature:        payment.signature,
		},
	)
	if err != nil {
		log.WithError(err).Error("Unable to store new payment channel state")
		return status.Error(codes.Internal, "unable to store new payment channel state")
	}

	return nil
}

func (h *escrowPaymentHandler) getSignerAddressFromPayment(payment *paymentData) (signer *common.Address, err error) {
	paymentHash := crypto.Keccak256(
		hashPrefix32Bytes,
		crypto.Keccak256(
			h.processor.escrowContractAddress.Bytes(),
			bigIntToBytes(payment.channelKey.Id),
			bigIntToBytes(payment.channelKey.Nonce),
			bigIntToBytes(payment.amount),
		),
	)

	publicKey, err := crypto.SigToPub(paymentHash, payment.signature)
	if err != nil {
		log.WithFields(log.Fields{
			"paymentHash": common.ToHex(paymentHash),
			"publicKey":   publicKey,
			"err":         err,
		}).Warn("Incorrect signature")
		return
	}

	keyOwnerAddress := crypto.PubkeyToAddress(*publicKey)
	return &keyOwnerAddress, nil
}

func bigIntToBytes(value *big.Int) []byte {
	return common.BigToHash(value).Bytes()
}

func (h *escrowPaymentHandler) getPaymentFromMetadata() (payment *paymentData, err error) {
	channelId, err := getBigInt(h.md, PaymentChannelIdHeader)
	if err != nil {
		return
	}

	channelNonce, err := getBigInt(h.md, PaymentChannelNonceHeader)
	if err != nil {
		return
	}

	amount, err := getBigInt(h.md, PaymentChannelAmountHeader)
	if err != nil {
		return
	}

	signature, err := getBytes(h.md, PaymentChannelSignatureHeader)
	if err != nil {
		return
	}

	return &paymentData{&PaymentChannelKey{channelId, channelNonce}, amount, signature}, nil
}

func (h *escrowPaymentHandler) completePayment(err error) error {
	return err
}
