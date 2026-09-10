import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { StatItem } from './StatItem'

describe('StatItem', () => {
  it('項目名と値を組にして出す', () => {
    render(
      <dl>
        <StatItem label="稼働状況">起動しています</StatItem>
      </dl>,
    )

    expect(screen.getByText('稼働状況')).toBeInTheDocument()
    expect(screen.getByText('起動しています')).toBeInTheDocument()
  })
})
