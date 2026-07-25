/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'
import { z } from 'zod'

const searchSchema = z.record(z.string(), z.unknown()).catch({})

export const Route = createFileRoute('/console/topup')({
  validateSearch: searchSchema,
  beforeLoad: ({ search }) => {
    throw redirect({ to: '/recharge', search })
  },
})
