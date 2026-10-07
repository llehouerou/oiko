import { useEffect, useState } from 'react'
import { connect } from './store'
import { Automations } from './automation/Automations'
import { Timeline } from './Timeline'
import { Home } from './Home'
import { Setup } from './Setup'
import { SignIn } from './SignIn'
import { Programs } from './Programs'
import { Persons } from './Persons'
import { Kiosks } from './Kiosks'
import { ApprovePairing } from './Pairing'
import { SignInLink } from './SignInLink'
import { Account } from './Account'
import { AuditPage } from './Audit'
import { screen, useMe } from './access'
export function App() {
  const me = useMe()
  const [page, setPage] = useState(location.hash)
  const [landing, setLanding] = useState(location.pathname) // a Setup, Sign-in or pairing link's page, its secret as the hash
  useEffect(() => {
    const follow = () => setPage(location.hash)
    addEventListener('hashchange', follow)
    return () => removeEventListener('hashchange', follow)
  }, [])
  const signedIn = !!me?.identity
  useEffect(() => (signedIn ? connect() : undefined), [signedIn])
  // Done with a link's page: the dashboard, the secret out of the address and the browser's history.
  const done = () => (history.replaceState(null, '', '/'), setPage(''), setLanding('/'))
  if (landing === '/setup') return <Setup onDone={done} />
  if (landing === '/sign-in') return <SignInLink onDone={done} />
  // Approving a Kiosk pairing needs an Admin signed in here first.
  if (landing === '/pair' && me?.identity) return <ApprovePairing onDone={done} />
  switch (screen(me, page)) {
    case null:
      return null
    case 'sign-in':
      return <SignIn me={me!} />
    case 'persons':
      return <Persons />
    case 'kiosks':
      return <Kiosks />
    case 'programs':
      return <Programs />
    case 'audit':
      return <AuditPage />
    case 'account':
      return <Account />
    case 'automations':
      return <Automations />
    case 'history':
      return <Timeline />
    case 'home':
      return <Home />
  }
}
