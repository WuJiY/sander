package middleware

import (
	"net/http"
	"sort"
	"time"

	"sander/db/nosql"
	"sander/logger"

	"github.com/labstack/echo"
	"github.com/polaris1119/goutils"
)

// CacheKeyAlgorithm .
type CacheKeyAlgorithm interface {
	GenCacheKey(echo.Context) string
}

// CacheKeyFunc .
type CacheKeyFunc func(echo.Context) string

// GenCacheKey .
func (c CacheKeyFunc) GenCacheKey(ctx echo.Context) string {
	return c(ctx)
}

// CacheKeyAlgorithmMap .
var CacheKeyAlgorithmMap = make(map[string]CacheKeyAlgorithm)

// LruCache .
var LruCache = nosql.DefaultLRUCache

// EchoCache 用于 echo 框架的缓存中间件。支持自定义 cache 数量
func EchoCache(cacheMaxEntryNum ...int) echo.MiddlewareFunc {

	if len(cacheMaxEntryNum) > 0 {
		LruCache = nosql.NewLRUCache(cacheMaxEntryNum[0])
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			req := ctx.Request()

			if req.Method() == "GET" {
				cacheKey := getCacheKey(ctx)

				if cacheKey != "" {
					ctx.Set(nosql.CacheKey, cacheKey)

					value, compressor, ok := LruCache.GetAndUnCompress(cacheKey)
					if ok {
						cacheData, ok := compressor.(*nosql.CacheData)
						if ok {

							// 1分钟更新一次
							if time.Now().Sub(cacheData.StoreTime) >= time.Minute {
								// TODO:雪崩问题处理
								goto NEXT
							}

							logger.Debug("cache hit:%+v,now:%+v", cacheData.StoreTime, time.Now())
							return ctx.JSONBlob(http.StatusOK, value)
						}
					}
				}
			}

		NEXT:
			if err := next(ctx); err != nil {
				return err
			}

			return nil
		}
	}
}

func getCacheKey(ctx echo.Context) string {
	cacheKey := ""
	if cacheKeyAlgorithm, ok := CacheKeyAlgorithmMap[ctx.Path()]; ok {
		// nil 表示不缓存
		if cacheKeyAlgorithm != nil {
			cacheKey = cacheKeyAlgorithm.GenCacheKey(ctx)
		}
	} else {
		cacheKey = defaultCacheKeyAlgorithm(ctx)
	}

	return cacheKey
}

func defaultCacheKeyAlgorithm(ctx echo.Context) string {
	filter := map[string]bool{
		"from":      true,
		"sign":      true,
		"nonce":     true,
		"timestamp": true,
	}
	form := ctx.FormParams()
	var keys = make([]string, 0, len(form))
	for key := range form {
		if _, ok := filter[key]; !ok {
			keys = append(keys, key)
		}
	}

	sort.Sort(sort.StringSlice(keys))

	buffer := goutils.NewBuffer()
	for _, k := range keys {
		buffer.Append(k).Append("=").Append(ctx.FormValue(k))
	}

	req := ctx.Request()
	return goutils.Md5(req.Method() + req.URL().Path() + buffer.String())
}
