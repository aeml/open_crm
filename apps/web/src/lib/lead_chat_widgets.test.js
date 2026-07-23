import { describe, expect, it } from 'vitest'
import { leadChatWidgetEmbedCode } from './lead_chat_widgets'

describe('lead chat widget embed contract', () => {
  it('escapes administrator-owned iframe attributes', () => {
    const embed = leadChatWidgetEmbedCode({
      publicId: 'cw_public',
      title: 'Talk" onload="alert(1)',
      position: 'inline'
    })

    expect(embed).toContain('title="Talk&quot; onload=&quot;alert(1)"')
    expect(embed).not.toContain('title="Talk" onload=')
    expect(embed).toContain('style="border:0;width:360px;')
  })
})
