/**
 * Flow: create → sign → authorize → capture → refund (EIP-3009)
 *
 * Run:
 *   set -a && source ../../.env && set +a
 *   npm test -- --testNamePattern="authorize"
 */

import {
  newClient, accountWallet, discoverPaymentMethod,
  createAndSign, signEip1559, signRefundPayload, pollUntilStatus, getEnv,
} from '../helpers.js'

test('authorize → capture → refund', async () => {
  const client     = newClient()
  const accWallet  = accountWallet()
  const pm         = await discoverPaymentMethod(client)
  const amount     = getEnv('AMOUNT', '1000000')

  // ── Create + sign ──────────────────────────────────────────────────────────
  console.log('→ creating payment and submitting payer signature')
  const { paymentId } = await createAndSign(client, pm, 'authorize', amount)
  console.log(`  payment_id=${paymentId}`)

  // ── Authorize ──────────────────────────────────────────────────────────────
  console.log('→ authorize/payload')
  const prep = await client.payments.authorizePayload(paymentId)
  expect(prep.unsignedTransaction).toBeTruthy()

  const signedAuth = await signEip1559(prep.unsignedTransaction, accWallet)
  await client.payments.authorize(paymentId, { signedTransaction: signedAuth })

  const auth = await pollUntilStatus(client, paymentId, ['authorized'], 'authorize')
  console.log(`  authorized — capturable=${auth.onChain?.capturableAmount}`)

  // ── Capture ────────────────────────────────────────────────────────────────
  console.log('→ capture/payload')
  const prepCap = await client.payments.capturePayload(paymentId, { amount })
  expect(prepCap.unsignedTransaction).toBeTruthy()

  const signedCap = await signEip1559(prepCap.unsignedTransaction, accWallet)
  await client.payments.capture(paymentId, { signedTransaction: signedCap })

  const cap = await pollUntilStatus(client, paymentId, ['captured', 'partially_captured'], 'capture')
  expect(cap.onChain?.capturableAmount).toBe('0')
  console.log(`  captured — status=${cap.status}`)

  // ── Refund (EIP-3009 two-phase) ────────────────────────────────────────────
  console.log('→ refund/payload phase 1')
  const phase1 = await client.payments.refundPayload(paymentId, { amount })
  expect(phase1.signingPayload).toBeTruthy()
  expect((phase1 as any).unsignedTransaction).toBeFalsy()

  console.log('→ signing EIP-3009 refund payload with TS SDK')
  const vrs = signRefundPayload(getEnv('ACCOUNT_PRIVATE_KEY'), phase1.signingPayload!)

  console.log('→ refund/payload phase 2')
  const phase2 = await client.payments.refundPayload(paymentId, { amount, ...vrs })
  expect(phase2.unsignedTransaction).toBeTruthy()

  console.log('→ submitting refund')
  const signedRef = await signEip1559(phase2.unsignedTransaction!, accWallet)
  await client.payments.refund(paymentId, { signedTransaction: signedRef })

  const final = await pollUntilStatus(client, paymentId, ['refunded', 'partially_refunded'], 'refund')
  expect(final.onChain?.refundableAmount).toBe('0')
  console.log(`  refunded — status=${final.status}`)
})

test('authorize → void', async () => {
  const client    = newClient()
  const accWallet = accountWallet()
  const pm        = await discoverPaymentMethod(client)
  const amount    = getEnv('AMOUNT', '1000000')

  const { paymentId } = await createAndSign(client, pm, 'authorize', amount)

  const prep = await client.payments.authorizePayload(paymentId)
  await client.payments.authorize(paymentId, {
    signedTransaction: await signEip1559(prep.unsignedTransaction, accWallet),
  })
  await pollUntilStatus(client, paymentId, ['authorized'], 'authorize')

  const prepVoid = await client.payments.voidPayload(paymentId)
  await client.payments.void(paymentId, {
    signedTransaction: await signEip1559(prepVoid.unsignedTransaction, accWallet),
  })
  const final = await pollUntilStatus(client, paymentId, ['voided'], 'void')
  expect(final.onChain?.capturableAmount).toBe('0')
  console.log(`  voided — status=${final.status}`)
})
