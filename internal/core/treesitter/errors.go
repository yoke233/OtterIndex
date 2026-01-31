package treesitter

import "errors"

var ErrDisabled = errors.New("treesitter disabled")
var ErrUnsupported = errors.New("treesitter unsupported file type")
