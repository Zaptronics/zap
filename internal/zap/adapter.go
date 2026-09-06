// SPDX-License-Identifier: Apache-2.0
package zap

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

func (p *Project) GenerateAdapter(c *Config, name string) error {
	if name == "" {
		name = "platform"
	}
	valid := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	if !valid.MatchString(name) {
		return fmt.Errorf("adapter name must be a C identifier")
	}
	root := c.Project.AdaptersDir
	if root == "" {
		root = "adapters"
	}
	out := root
	if !filepath.IsAbs(out) {
		out = filepath.Join(p.Root, out)
	}
	out = filepath.Join(out, name)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	h := fmt.Sprintf(`// SPDX-License-Identifier: Apache-2.0
#pragma once

#include <zaptronics/platform/i2c.h>
#include <zaptronics/platform/spi.h>
#include <zaptronics/platform/lock.h>
#include <zaptronics/platform/time.h>
#include <zaptronics/modules/ztm_pwm.h>
#include <zaptronics/modules/ztm_adc.h>

typedef struct {
    void *platform_context;
    zap_i2c_bus_t i2c;
    zap_spi_device_t spi;
    zap_lock_t operation_lock;
    zap_time_t time;
} zap_%[1]s_adapter_t;

int zap_%[1]s_adapter_init(zap_%[1]s_adapter_t *adapter, void *platform_context);
void zap_%[1]s_ztm_pwm_config(ztm_pwm_config_t *config, zap_%[1]s_adapter_t *adapter);
void zap_%[1]s_ztm_adc_config(ztm_adc_config_t *config, zap_%[1]s_adapter_t *adapter);
`, name)
	cc := fmt.Sprintf(`// SPDX-License-Identifier: Apache-2.0
#include "zap_%[1]s_adapter.h"
#include <string.h>

/* Replace these TODO callbacks with the SDK/RTOS primitives for your platform. */
static int platform_i2c_write(void *context, uint8_t address_7bit,
                              const uint8_t *data, size_t length, uint32_t timeout_us)
{
    (void)context; (void)address_7bit; (void)data; (void)length; (void)timeout_us;
    return ZAP_I2C_E_UNSUPPORTED;
}

static int platform_i2c_write_read(void *context, uint8_t address_7bit,
                                   const uint8_t *write_data, size_t write_length,
                                   uint8_t *read_data, size_t read_length,
                                   uint32_t timeout_us)
{
    (void)context; (void)address_7bit; (void)write_data; (void)write_length;
    (void)read_data; (void)read_length; (void)timeout_us;
    return ZAP_I2C_E_UNSUPPORTED;
}

static int platform_i2c_probe(void *context, uint8_t address_7bit, uint32_t timeout_us)
{
    (void)context; (void)address_7bit; (void)timeout_us;
    return ZAP_I2C_E_UNSUPPORTED;
}

static int platform_spi_begin(void *context, uint32_t timeout_us)
{ (void)context; (void)timeout_us; return ZAP_SPI_E_UNSUPPORTED; }
static int platform_spi_transfer(void *context, const uint8_t *tx, uint8_t *rx,
                                 size_t length, uint32_t timeout_us)
{ (void)context; (void)tx; (void)rx; (void)length; (void)timeout_us; return ZAP_SPI_E_UNSUPPORTED; }
static void platform_spi_end(void *context) { (void)context; }

static int platform_lock(void *context, uint32_t timeout_us)
{ (void)context; (void)timeout_us; return 0; }
static void platform_unlock(void *context) { (void)context; }
static void platform_delay_ms(void *context, uint32_t ms) { (void)context; (void)ms; }
static void platform_delay_us(void *context, uint32_t us) { (void)context; (void)us; }
static uint64_t platform_now_us(void *context) { (void)context; return 0; }

static const zap_i2c_bus_ops_t g_i2c_ops = {
    .write = platform_i2c_write,
    .write_read = platform_i2c_write_read,
    .probe = platform_i2c_probe,
};
static const zap_spi_device_ops_t g_spi_ops = {
    .begin = platform_spi_begin,
    .transfer = platform_spi_transfer,
    .end = platform_spi_end,
};

int zap_%[1]s_adapter_init(zap_%[1]s_adapter_t *adapter, void *platform_context)
{
    if (adapter == NULL) return -1;
    memset(adapter, 0, sizeof(*adapter));
    adapter->platform_context = platform_context;
    adapter->i2c.context = platform_context;
    adapter->i2c.ops = &g_i2c_ops;
    adapter->i2c.timeout_us = ZAP_I2C_DEFAULT_TIMEOUT_US;
    adapter->spi.context = platform_context;
    adapter->spi.ops = &g_spi_ops;
    adapter->spi.timeout_us = ZAP_SPI_DEFAULT_TIMEOUT_US;
    adapter->operation_lock.context = platform_context;
    adapter->operation_lock.acquire = platform_lock;
    adapter->operation_lock.release = platform_unlock;
    adapter->time.context = platform_context;
    adapter->time.delay_ms = platform_delay_ms;
    adapter->time.delay_us = platform_delay_us;
    adapter->time.now_us = platform_now_us;
    return 0;
}

void zap_%[1]s_ztm_pwm_config(ztm_pwm_config_t *config, zap_%[1]s_adapter_t *adapter)
{
    if (config == NULL || adapter == NULL) return;
    memset(config, 0, sizeof(*config));
    config->i2c = &adapter->i2c;
    config->time = &adapter->time;
    /* TODO: set board power/IRQ/event callbacks. */
}

void zap_%[1]s_ztm_adc_config(ztm_adc_config_t *config, zap_%[1]s_adapter_t *adapter)
{
    if (config == NULL || adapter == NULL) return;
    memset(config, 0, sizeof(*config));
    config->spi = &adapter->spi;
    config->time = &adapter->time;
    config->lock = &adapter->operation_lock;
}
`, name)
	readme := fmt.Sprintf("# %s ZapEE adapter\n\nGenerated by `zap adapter %s`. Replace the TODO transport/time/locking callbacks with your MCU SDK or RTOS implementation. Remove unused interfaces if this project only needs I2C or SPI.\n", name, name)
	for file, data := range map[string]string{"zap_" + name + "_adapter.h": h, "zap_" + name + "_adapter.c": cc, "README.md": readme} {
		path := filepath.Join(out, file)
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists", path)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("Created adapter skeleton in %s\n", out)
	return nil
}
