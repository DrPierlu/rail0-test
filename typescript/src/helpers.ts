/**
 * Shared helpers for all TypeScript integration tests.
 */

import { ethers } from 'ethers'
import { Rail0Client, signPayment, type CreatePaymentResponse, type PaymentMethod } from '@rail0/sdk'

export const POLL_TIMEOUT  = 120_000 // ms
export const POLL_INTERVAL = 2_000   // ms

// ── Environment ──────────────────────────────────────────────────────────────

export function getEnv(key: string, defaultValue?: string): string {
  const v = process.env[key] ?? defaultValue
  if (!v) throw new Error(`required env var ${key} is not set`)
  return v
}

// ── Client ───────────────────────────────────────────────────────────────────

export function newClient(): Rail0Client {
  return new Rail0Client({ baseUrl: getEnv('RAIL0_API_URL', 'http://localhost:4567') })
}

// ── Wallets ───────────────────────────────────────────────────────────────────

export function buyerWallet(): ethers.Wallet {
  return new ethers.Wallet(getEnv('BUYER_PRIVATE_KEY'))
}

export function accountWallet(): ethers.Wallet {
  return new ethers.Wallet(getEnv('ACCOUNT_PRIVATE_KEY'))
}

// ── Payment method discovery ─────────────────────────────────────────────────

export async function discoverPaymentMethod(client: Rail0Client): Promise<PaymentMethod> {
  const accountId = getEnv('ACCOUNT_ID')
  const chainSlug = getEnv('CHAIN_SLUG', 'arc-testnet')
  const symbol    = getEnv('TOKEN_SYMBOL', 'USDC')

  const methods = await client.accounts.paymentMethods(accountId)
  const pm = methods.find((m) => m.chainSlug === chainSlug && m.tokenSymbol === symbol)
  if (!pm) throw new Error(`no ${symbol} payment method on ${chainSlug}`)
  return pm
}

// ── Create + sign ─────────────────────────────────────────────────────────────

export async function createAndSign(
  client: Rail0Client,
  pm: PaymentMethod,
  mode: 'authorize' | 'charge',
  amount = getEnv('AMOUNT', '1000000'),
): Promise<{ paymentId: string; createResp: CreatePaymentResponse }> {
  const chainId = parseInt(getEnv('CHAIN_ID', '5042002'), 10)
  const buyer   = buyerWallet()

  const createResp = await client.payments.createPayment({
    payment: {
      payer:  buyer.address.toLowerCase(),
      payee:  pm.walletAddress,
      token:  pm.tokenAddress,
      amount,
    },
    chainId,
    mode,
  })

  const sig = signPayment(getEnv('BUYER_PRIVATE_KEY') as `0x${string}`, createResp)
  const signature = `0x${sig.r.slice(2)}${sig.s.slice(2)}${sig.v.toString(16).padStart(2, '0')}`

  const signResp = await client.payments.sign(createResp.rail0Id, { signature })
  if (signResp.status !== 'signature_stored') {
    throw new Error(`unexpected sign status: ${signResp.status}`)
  }

  return { paymentId: createResp.rail0Id, createResp }
}

// ── EIP-1559 tx signing ───────────────────────────────────────────────────────

export async function signEip1559(unsignedHex: string, wallet: ethers.Wallet): Promise<string> {
  const tx = ethers.Transaction.from(unsignedHex)
  return wallet.signTransaction(tx)
}

// ── EIP-3009 refund signing ───────────────────────────────────────────────────

export function signRefundPayload(
  privateKey: string,
  signingPayload: NonNullable<Awaited<ReturnType<Rail0Client['payments']['refundPayload']>>['signingPayload']>,
): { v: number; r: string; s: string } {
  // Refund uses ReceiveWithAuthorization — same EIP-712 structure as authorize.
  // We reuse signPayment by constructing a compatible CreatePaymentResponse shape.
  const fakeResp = {
    signingPayload,
    rail0Id:       '',
    configHash:    '',
    rail0Contract: '',
    payment:       {} as any,
    chainId:       0,
    mode:          'authorize' as const,
  } satisfies CreatePaymentResponse

  const sig = signPayment(privateKey as `0x${string}`, fakeResp)
  return { v: sig.v, r: sig.r, s: sig.s }
}

// ── Polling ───────────────────────────────────────────────────────────────────

export async function pollUntilStatus(
  client: Rail0Client,
  paymentId: string,
  expected: string[],
  waitingFor: string,
) {
  const deadline = Date.now() + POLL_TIMEOUT
  while (true) {
    const state = await client.payments.get(paymentId)
    console.log(`  [poll] ${waitingFor}: status=${state.status}`)
    if (expected.includes(state.status)) return state
    if (state.status === 'failed') {
      throw new Error(`payment failed: ${state.failureCode} — ${state.failureMessage}`)
    }
    if (Date.now() >= deadline) {
      throw new Error(`timed out waiting for [${expected.join(', ')}] (last: ${state.status})`)
    }
    await new Promise((r) => setTimeout(r, POLL_INTERVAL))
  }
}
