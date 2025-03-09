package art

import (
	"fmt"
)

// mixin to help deserialize interfaces like Inode, node.
// (from when we tried serializing inner nodes
// to disk. this turned out to be a bulky, bad idea!)
type dserzIfaceMixin struct{}

func (up *dserzIfaceMixin) NewValueAsInterface(zid int64, typename string) interface{} {
	//vv("request for typename '%v' zid = %v", typename, zid)
	switch typename {
	case "node4":
		//vv("returning node4")
		return &node4{}
	case "node16":
		return &node16{}
	case "node48":
		return &node48{}
	case "node256":
		return &node256{}
	case "Leaf":
		return &Leaf{}
	case "Inner":
		return &Inner{}
	}
	panic(fmt.Sprintf("arg! add case for '%v'", typename))
	return nil
}
