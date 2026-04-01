package lzio

/*
** $Id: lzio.go $
** Buffered streams
** Ported from lzio.h and lzio.c
*/

import (
	"github.com/akzj/go-lua/internal/lstate"
)

/*
** End of stream marker
 */
const EOZ = -1

/*
** Minimum buffer size for lexical analyzer
 */
const MinBuffer = 32

/*
** Lua reader function type
** Matches C signature: lua_Reader(lua_State *L, void *data, size_t *size)
 */
type Reader func(L *lstate.LuaState, data interface{}, size *int64) []byte

/*
** ZIO - input stream structure
 */
type ZIO struct {
	N      int64  // bytes still unread
	P      []byte // current position in buffer
	Reader Reader // reader function
	Data   interface{}
	L      *lstate.LuaState
}

/*
** Mbuffer - buffer for lexical analyzer
 */
type Mbuffer struct {
	Buffer   []byte
	N        int64 // current length
	BuffSize int64 // allocated size
}

/*
** Zgetc - get next character from stream
** Macro: ((z)->n-- > 0 ? cast_uchar(*(z)->p++) : luaZ_fill(z))
 */
func Zgetc(z *ZIO) int {
	if z.N > 0 {
		c := int(z.P[0])
		z.P = z.P[1:]
		z.N--
		return c
	}
	return Fill(z)
}

/*
** InitBuffer - initialize buffer
** #define luaZ_initbuffer(L, buff) ((buff)->buffer = NULL, (buff)->buffsize = 0)
 */
func InitBuffer(buff *Mbuffer) {
	buff.Buffer = nil
	buff.BuffSize = 0
	buff.N = 0
}

/*
** Resize buffer
** #define luaZ_resizebuffer(L, buff, size) \
**     ((buff)->buffer = luaM_reallocvchar(L, (buff)->buffer, \
**         (buff)->buffsize, size), \
**     (buff)->buffsize = size)
 */
func ResizeBuffer(L *lstate.LuaState, buff *Mbuffer, size int64) {
	buff.Buffer = ReallocBytes(L, buff.Buffer, buff.BuffSize, size)
	buff.BuffSize = size
}

/*
** Free buffer
** #define luaZ_freebuffer(L, buff) luaZ_resizebuffer(L, buff, 0)
 */
func FreeBuffer(L *lstate.LuaState, buff *Mbuffer) {
	ResizeBuffer(L, buff, 0)
}

/*
** Get buffer pointer
** #define luaZ_buffer(buff) ((buff)->buffer)
 */
func Buffer(buff *Mbuffer) []byte {
	return buff.Buffer
}

/*
** Get buffer size
** #define luaZ_sizebuffer(buff) ((buff)->buffsize)
 */
func SizeBuffer(buff *Mbuffer) int64 {
	return buff.BuffSize
}

/*
** Get buffer length
** #define luaZ_bufflen(buff) ((buff)->n)
 */
func BuffLen(buff *Mbuffer) int64 {
	return buff.N
}

/*
** Remove characters from end of buffer
** #define luaZ_buffremove(buff,i) ((buff)->n -= cast_sizet(i))
 */
func BuffRemove(buff *Mbuffer, i int64) {
	buff.N -= i
}

/*
** Reset buffer
** #define luaZ_resetbuffer(buff) ((buff)->n = 0)
 */
func ResetBuffer(buff *Mbuffer) {
	buff.N = 0
}

/*
** Init ZIO stream
 */
func Init(L *lstate.LuaState, z *ZIO, reader Reader, data interface{}) {
	z.L = L
	z.Reader = reader
	z.Data = data
	z.N = 0
	z.P = nil
}

/*
** Fill buffer - read more bytes from reader
** int luaZ_fill (ZIO *z) {
**   size_t size;
**   lua_State *L = z->L;
**   const char *buff;
**   lua_unlock(L);
**   buff = z->reader(L, z->data, &size);
**   lua_lock(L);
**   if (buff == NULL || size == 0)
**     return EOZ;
**   z->n = size - 1;  // discount char being returned
**   z->p = buff;
**   return cast_uchar(*(z->p++));
** }
 */
func Fill(z *ZIO) int {
	L := z.L
	var size int64 = 0
	buff := z.Reader(L, z.Data, &size)
	if buff == nil || len(buff) == 0 || size == 0 {
		return EOZ
	}
	z.N = size - 1 // discount char being returned
	z.P = buff
	if len(buff) > 0 {
		c := int(buff[0])
		z.P = buff[1:]
		return c
	}
	return EOZ
}

/*
** checkbuffer - ensure buffer has data
** static int checkbuffer (ZIO *z) {
**   if (z->n == 0) {
**     if (luaZ_fill(z) == EOZ)
**       return 0;
**     else {
**       z->n++;
**       z->p--;
**     }
**   }
**   return 1;
** }
 */
func checkBuffer(z *ZIO) bool {
	if z.N <= 0 {
		c := Fill(z)
		if c == EOZ {
			return false
		}
		z.N++
		if len(z.P) > 0 {
			z.P = z.P[1:]
		}
		z.P = append([]byte{byte(c)}, z.P...)
	}
	return true
}

/*
** luaZ_read - read n bytes from stream
** size_t luaZ_read (ZIO* z, void *b, size_t n)
 */
func Read(z *ZIO, b []byte, n int64) int64 {
	if n == 0 {
		return 0
	}
	for {
		if !checkBuffer(z) {
			return n // no more input
		}
		m := n
		if z.N < m {
			m = z.N
		}
		copy(b, z.P[:m])
		z.P = z.P[m:]
		z.N -= m
		b = b[m:]
		n -= m
		if n == 0 {
			return 0
		}
	}
}

/*
** luaZ_getaddr - get block address
** const void *luaZ_getaddr (ZIO* z, size_t n)
 */
func GetAddr(z *ZIO, n int64) []byte {
	if !checkBuffer(z) {
		return nil
	}
	if z.N < n {
		return nil
	}
	res := z.P[:n]
	z.P = z.P[n:]
	z.N -= n
	return res
}

// Helper function for reallocating byte slices
func ReallocBytes(L *lstate.LuaState, old []byte, oldsize, size int64) []byte {
	if size == 0 {
		return nil
	}
	if old == nil {
		return make([]byte, size)
	}
	result := make([]byte, size)
	copy(result, old)
	return result
}

// For compatibility with lmem package
func Malloc(L *lstate.LuaState, size uint64) []byte {
	return make([]byte, size)
}

// Cast to byte
func cast_uchar(c int) byte {
	return byte(c)
}

// Cast size_t
func cast_sizet(i int64) uint64 {
	return uint64(i)
}
