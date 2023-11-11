package headers

import (
	"fmt"

	"github.com/valyala/fasthttp"
)

func WriteBearerToken(req *fasthttp.Request, token string) {
	req.Header.Set(Authorization, fmt.Sprintf("Bearer %s", token))
}
