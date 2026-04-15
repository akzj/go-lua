CC=gcc
CFLAGS=-O2 -Wall -std=c99 -DLUA_USE_LINUX -DLUA_USE_READLINE
LDFLAGS=-lm -ldl -lreadline

SRCS=$(wildcard *.c)
OBJS=$(SRCS:.c=.o)

lua: $(OBJS)
	$(CC) -o $@ $^ $(LDFLAGS)

clean:
	rm -f *.o lua
