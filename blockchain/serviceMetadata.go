package blockchain

import (
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/singnet/snet-daemon/config"
	"github.com/singnet/snet-daemon/ipfsutils"
	log "github.com/sirupsen/logrus"
	"math/big"
	"strings"
)

const IpfsPrefix = "ipfs"

type ServiceMetadata struct {
	Version                    int    `json:"version"`
	DisplayName                string `json:"display_name"`
	Encoding                   string `json:"encoding"`
	ServiceType                string `json:"service_type"`
	PaymentExpirationThreshold int64  `json:"payment_expiration_threshold"`
	ModelIpfsHash              string `json:"model_ipfs_hash"`
	MpeAddress                 string `json:"mpe_address"`
	Pricing                    struct {
		PriceModel  string   `json:"price_model"`
		PriceInCogs *big.Int `json:"price_in_cogs"`
	} `json:"pricing"`
	Groups []struct {
		GroupName      string `json:"group_name"`
		GroupID        string `json:"group_id"`
		PaymentAddress string `json:"payment_address"`
	} `json:"groups"`
	Endpoints []struct {
		GroupName string `json:"group_name"`
		Endpoint  string `json:"endpoint"`
	} `json:"endpoints"`
	daemonReplicaGroupID    [32]byte
	daemonGroupName         string
	daemonEndPoint          string
	recipientPaymentAddress common.Address
}

func getRegistryAddressKey() common.Address {
	address := config.GetString(config.RegistryAddressKey)
	return common.HexToAddress(address)
}

func ServiceMetaData() *ServiceMetadata {

	var metadata *ServiceMetadata
	var err error
	if config.GetBool(config.BlockchainEnabledKey) {
		ipfsHash := string(getMetaDataUrifromRegistry())
		metadata, err = GetServiceMetaDataFromIPFS(FormatHash(ipfsHash))
		if err != nil {
			log.WithError(err).WithField("IPFSHashOfFile", ipfsHash).
				Panic("error On Retrieving service metadata file from IPFS")
		}
	} else {
		//TO DO Load the metaData from a test JSON configuration here
		strJson := ""
		metadata, err = InitServiceMetaDataFromJson(strJson)
		if err != nil {
			log.WithError(err).Panic("error on parsing service metadata configured")
		}
	}

	return metadata
}

func getMetaDataUrifromRegistry() []byte {
	ethClient, err := GetEthereumClient()
	registryContractAddress := getRegistryAddressKey()
	reg, err := NewRegistryCaller(registryContractAddress, ethClient.EthClient)
	if err != nil {
		log.WithError(err).WithField("registryContractAddress", registryContractAddress).
			Panic("Error instantiating Registry contract for the given Contract Address")
	}
	orgName := StringToBytes32(config.GetString(config.OrganizationName))
	serviceName := StringToBytes32(config.GetString(config.ServiceName))

	serviceRegistration, err := reg.GetServiceRegistrationByName(nil, orgName, serviceName)
	if err != nil {
		log.WithError(err).WithField("OrganizationName", config.GetString(config.OrganizationName)).
			WithField("ServiceName", config.GetString(config.ServiceName)).
			Panic("Error Retrieving contract details for the Given Organization and Service Name ")
	}
	return serviceRegistration.MetadataURI[:]

}

func GetServiceMetaDataFromIPFS(hash string) (*ServiceMetadata, error) {
	jsondata := ipfsutils.GetIpfsFile(hash)
	return InitServiceMetaDataFromJson(jsondata)
}

func InitServiceMetaDataFromJson(jsonData string) (*ServiceMetadata, error) {
	metaData := new(ServiceMetadata)
	err := json.Unmarshal([]byte(jsonData), &metaData)
	if err != nil {
		log.WithError(err).WithField("jsondata", jsonData).
			Panic("Parsing the service metadata JSON failed")
	}
	err = setDaemonEndPoint(metaData)
	if err != nil {
		return nil, err
	}
	err = setDaemonGroupName(metaData)
	if err != nil {
		return nil, err
	}
	err = setDaemonGroupIDAndPaymentAddress(metaData)
	if err != nil {
		return nil, err
	}
	return metaData, err
}

func (metaData *ServiceMetadata) SetServiceMetaData(hash string) *ServiceMetadata {
	jsondata := ipfsutils.GetIpfsFile(hash)
	return SetServiceMetaDataThroughJSON(jsondata)
}

func SetServiceMetaDataThroughJSON(jsondata string) *ServiceMetadata {
	metaData := new(ServiceMetadata)
	json.Unmarshal([]byte(jsondata), &metaData)
	return metaData
}

func setDaemonEndPoint(metaData *ServiceMetadata) error {
	metaData.daemonEndPoint = config.GetString(config.DaemonEndPoint)
	if len(metaData.daemonEndPoint) == 0 {
		log.WithField("daemonEndPoint", metaData.daemonEndPoint)
		return fmt.Errorf("check the Daemon End Point in the config")
	}
	return nil
}

func setDaemonGroupName(metaData *ServiceMetadata) error {
	for _, endpoints := range metaData.Endpoints {
		if strings.Compare(metaData.daemonEndPoint, endpoints.Endpoint) == 0 {
			metaData.daemonGroupName = endpoints.GroupName
			return nil
		}
	}
	log.WithField("DaemonEndPoint", metaData.daemonEndPoint)
	return fmt.Errorf("unable to determine Daemon Group Name, DaemonEndPoint %s", metaData.daemonEndPoint)
}

func setDaemonGroupIDAndPaymentAddress(metaData *ServiceMetadata) error {
	groupName := metaData.GetDaemonGroupName()
	for _, group := range metaData.Groups {
		if strings.Compare(groupName, group.GroupName) == 0 {
			metaData.daemonReplicaGroupID = convertBase64Encoding(group.GroupID)
			metaData.recipientPaymentAddress = common.HexToAddress(group.PaymentAddress)
			return nil
		}
	}
	log.WithField("groupName", groupName)
	return fmt.Errorf("unable to determine the Daemon Group ID or the Recipient Payment Address, Daemon Group Name")

}

func (metaData *ServiceMetadata) GetDaemonEndPoint() string {
	return metaData.daemonEndPoint
}

func (metaData *ServiceMetadata) GetMpeAddress() common.Address {
	return common.HexToAddress(metaData.MpeAddress)
}

func (metaData *ServiceMetadata) GetPaymentExpirationThreshold() int64 {
	return metaData.PaymentExpirationThreshold
}

func (metaData *ServiceMetadata) GetPriceInCogs() *big.Int {
	return metaData.Pricing.PriceInCogs
}

func (metaData *ServiceMetadata) GetDaemonGroupName() string {
	return metaData.daemonGroupName
}
func (metaData *ServiceMetadata) GetWireEncoding() string {
	return metaData.Encoding
}

func (metaData *ServiceMetadata) GetVersion() int {
	return metaData.Version
}

func (metaData *ServiceMetadata) GetServiceType() string {
	return metaData.ServiceType
}

func (metaData *ServiceMetadata) GetDisplayName() string {
	return metaData.DisplayName

}

func (metaData *ServiceMetadata) GetDaemonGroupID() [32]byte {
	return metaData.daemonReplicaGroupID
}

func (metaData *ServiceMetadata) GetPaymentAddress() common.Address {
	return metaData.recipientPaymentAddress
}
