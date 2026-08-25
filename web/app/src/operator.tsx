import React from "react";
import { loadMe, type Operator } from "./api";

// Operator session state lives above the router so signing in on one
// page carries to every write surface. The token is intentionally kept
// in memory only (ADR-0016: paste-a-bearer-token, no storage).
type OperatorSession = {
  operator: Operator | null;
  token: string | null;
  signIn: (token: string) => Promise<void>;
  signOut: () => void;
};

const OperatorContext = React.createContext<OperatorSession | null>(null);

export function OperatorProvider({ children }: { children: React.ReactNode }) {
  const [operator, setOperator] = React.useState<Operator | null>(null);
  const [token, setToken] = React.useState<string | null>(null);

  const value = React.useMemo<OperatorSession>(
    () => ({
      operator,
      token,
      signIn: async (signedInToken: string) => {
        const me = await loadMe(signedInToken);
        setToken(signedInToken);
        setOperator(me);
      },
      signOut: () => {
        setToken(null);
        setOperator(null);
      },
    }),
    [operator, token],
  );

  return <OperatorContext.Provider value={value}>{children}</OperatorContext.Provider>;
}

export function useOperator(): OperatorSession {
  const session = React.useContext(OperatorContext);
  if (!session) {
    throw new Error("useOperator must be used within OperatorProvider");
  }
  return session;
}
