/**
 * Flow: create (mode=charge) → sign → charge
 *
 * Run:
 *   set -a && source ../../.env && set +a
 *   npm test -- --testNamePattern="charge"
 */

import {
  newClient, accountWallet, discoverPaymentMethod,
  createAndSign, signEip1559, pollUntilStatus, getEnv,
} from '../helpers.js'

test('charge (one-shot)', async () => {
  const client    = newClient()
  const accWallet = accountWallet()
  const pm        = await discoverPaymentMethod(client)
  const amount    = getEnv('AMOUNT', '1000000')

  console.log('→ creating payment (mode=charge) and submitting payer signature')
  const { paymentId } = await createAndSign(client, pm, 'charge', amount)
  console.log(`  payment_id=${paymentId}`)

  console.log('→ charge/payload')
  const prep = await client.payments.chargePayload(paymentId)
  expect(prep.unsignedTransaction).toBeTruthy()

  const signed = await signEip1559(prep.unsignedTransaction, accWallet)
  await client.payments.charge(paymentId, { signedTransaction: signed })

  const final = await pollUntilStatus(client, paymentId, ['charged'], 'charge')
  console.log(`  charged — status=${final.status}`)
})
