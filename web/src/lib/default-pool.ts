/*
Copyright (C) 2026 xju-api contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

/**
 * Operational redemption-code rate shown to users.
 *
 * Redemption codes store quota rather than a CNY payment amount, so this
 * constant is presentation guidance and must not be used to change the global
 * quota unit, model pricing, or the Default-pool usage multiplier.
 */
export const DEFAULT_POOL_USD_CREDIT_PER_CNY = 100

export const DEFAULT_POOL_REDEMPTION_RATE_LABEL = `¥1 = $${DEFAULT_POOL_USD_CREDIT_PER_CNY}`
