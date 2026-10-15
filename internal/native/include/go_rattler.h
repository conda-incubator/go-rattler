#ifndef GO_RATTLER_H
#define GO_RATTLER_H

#include <stddef.h>
#include <stdint.h>

int32_t go_rattler_compare_versions(const uint8_t *a, size_t a_len,
                                    const uint8_t *b, size_t b_len,
                                    int32_t *comparison);

int32_t go_rattler_lock_environment_counts(const uint8_t *lock_path,
                                            size_t lock_path_len,
                                            const uint8_t *environment,
                                            size_t environment_len,
                                            const uint8_t *platform,
                                            size_t platform_len,
                                            uint32_t *conda_count,
                                            uint32_t *pypi_count);

#endif
