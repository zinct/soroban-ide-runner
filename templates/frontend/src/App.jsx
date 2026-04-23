import React, { useState } from 'react';
import { isConnected, getAddress } from "@stellar/freighter-api";
import { Globe, Shield, Wallet, Zap } from "lucide-react";

function App() {
  const [address, setAddress] = useState("");
  const [connecting, setConnecting] = useState(false);

  const connectWallet = async () => {
    setConnecting(true);
    try {
      if (await isConnected()) {
        const { address: addr } = await getAddress();
        setAddress(addr);
      } else {
        alert("Please install Freighter wallet");
      }
    } catch (err) {
      console.error(err);
    } finally {
      setConnecting(false);
    }
  };

  return (
    <div className="min-h-screen bg-slate-950 text-slate-50 flex flex-col items-center justify-center p-4">
      <div className="max-w-md w-full space-y-8 bg-slate-900/50 backdrop-blur-xl border border-slate-800 p-8 rounded-2xl shadow-2xl">
        <div className="text-center">
          <div className="inline-flex p-3 bg-brand-primary/10 rounded-xl mb-4 text-brand-primary">
            <Zap size={32} fill="currentColor" />
          </div>
          <h1 className="text-3xl font-bold tracking-tight">Soroban Studio</h1>
          <p className="mt-2 text-slate-400">Welcome to your new Stellar frontend</p>
        </div>

        <div className="space-y-4">
          {!address ? (
            <button
              onClick={connectWallet}
              disabled={connecting}
              className="w-full flex items-center justify-center gap-2 bg-brand-primary hover:bg-brand-hover text-white font-semibold py-3 px-4 rounded-xl transition-all active:scale-95 disabled:opacity-50"
            >
              <Wallet size={20} />
              {connecting ? "Connecting..." : "Connect Freighter"}
            </button>
          ) : (
            <div className="space-y-4">
              <div className="p-4 bg-slate-800/50 rounded-lg border border-slate-700">
                <p className="text-xs text-slate-500 uppercase font-semibold mb-1">Account Address</p>
                <p className="text-sm font-mono break-all text-brand-primary">{address}</p>
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="p-4 bg-slate-800/50 rounded-lg border border-slate-700">
                  <Shield size={20} className="text-slate-400 mb-2" />
                  <p className="text-sm font-medium">Safe Mode</p>
                </div>
                <div className="p-4 bg-slate-800/50 rounded-lg border border-slate-700">
                  <Globe size={20} className="text-slate-400 mb-2" />
                  <p className="text-sm font-medium">Testnet</p>
                </div>
              </div>
            </div>
          )}
        </div>

        <div className="pt-6 border-t border-slate-800 text-center">
          <p className="text-xs text-slate-500">
            Edit <code className="text-brand-primary">src/App.jsx</code> to get started
          </p>
        </div>
      </div>
    </div>
  );
}

export default App;
