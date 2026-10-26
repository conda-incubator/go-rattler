#ifndef GO_RATTLER_H
#define GO_RATTLER_H

#include <stddef.h>
#include <stdint.h>

/* Owned output buffer. Rust allocates `data` as an exact-length boxed byte
 * slice; a zero-length result uses data == NULL. Zero-initialize before the call
 * and release with go_rattler_json_free after every call, including errors. */
typedef struct JsonBuffer {
  uint8_t *data;
  size_t len;
} JsonBuffer;

/* Status codes for go_rattler_json. */
#define GO_RATTLER_STATUS_OK 0
#define GO_RATTLER_STATUS_DOMAIN_ERROR 2
#define GO_RATTLER_STATUS_INVALID_INPUT 3
#define GO_RATTLER_STATUS_INTERNAL 4

/* Runs one JSON operation. `output` must be non-null and must be empty
 * (data == NULL, len == 0). Input may be NULL only when input_len == 0.
 * For a valid output pointer/empty buffer, output holds JSON on success and
 * {"error":"..."} on failure. Rejected output pointers/buffers are untouched. */
int32_t go_rattler_json(uint32_t operation, const uint8_t *input,
                        size_t input_len, JsonBuffer *output);

/* Frees a buffer from go_rattler_json and zeroes it. NULL and empty buffers
 * are no-ops; repeated frees of the same struct are safe. */
void go_rattler_json_free(JsonBuffer *buffer);

#endif
