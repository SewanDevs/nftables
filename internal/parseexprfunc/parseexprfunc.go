package parseexprfunc

import (
	"github.com/SewanDevs/netlink"
)

var (
	ParseExprBytesFunc func(fam byte, ad *netlink.AttributeDecoder, b []byte) ([]interface{}, error)
	ParseExprMsgFunc   func(fam byte, b []byte) ([]interface{}, error)
)
