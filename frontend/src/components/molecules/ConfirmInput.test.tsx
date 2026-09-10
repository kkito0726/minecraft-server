import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'

import { ConfirmInput } from './ConfirmInput'

function Harness({ expected }: { expected: string }) {
  const [value, setValue] = useState('')
  return <ConfirmInput id="confirm" expected={expected} value={value} onChange={setValue} />
}

describe('ConfirmInput', () => {
  it('打ち込むべき名前を示す', () => {
    render(<Harness expected="world" />)
    expect(screen.getAllByText('world').length).toBeGreaterThan(0)
  })

  // 何も入れていない段階で赤くすると、開いた直後から警告が出る。
  it('未入力では間違いを指摘しない', () => {
    render(<Harness expected="world" />)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('食い違っている間は理由を出す', async () => {
    render(<Harness expected="world" />)

    await userEvent.type(screen.getByRole('textbox'), 'wor')
    expect(await screen.findByRole('alert')).toHaveTextContent('一致していません')
  })

  it('一致すれば指摘が消える', async () => {
    render(<Harness expected="world" />)

    await userEvent.type(screen.getByRole('textbox'), 'world')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
