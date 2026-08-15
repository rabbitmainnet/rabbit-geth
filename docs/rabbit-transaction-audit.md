# Rabbit Chain — laboratório e auditoria de transações

Este pacote acrescenta um teste de integração sem alterar o consenso e sem modificar os
arquivos oficiais de genesis.

## Por que o laboratório precisa de um saldo líquido

As recompensas dos blocos 1 a 100.000 são corretamente enviadas ao Reward Locker. Por isso,
as contas dos produtores não têm RAB líquido para pagar uma transação durante um laboratório
curto. O script do laboratório cria uma cópia temporária do genesis em
`/tmp/rabbit-20nodes/genesis-runtime.json` e fornece 1.000 RAB à conta do node20 somente nessa
rede descartável.

Os arquivos `networks/rabbit-devnet/genesis.json` e `networks/rabbit-mainnet/genesis.json` não
são alterados. O laboratório anterior também é movido para um diretório de backup antes da
reinicialização.

## O que é validado

O programa `cmd/rabbit-tx-audit` assina localmente, com o keystore criptografado do laboratório,
uma transferência EIP-1559 de 1 RAB do node20 para o node2 e envia somente a transação assinada
por IPC. Depois ele valida no estado histórico do node1:

- receipt bem-sucedido e inclusão no bloco canônico;
- nonce e débito exato do remetente;
- crédito exato do destinatário;
- gas usado e taxa efetiva;
- parte da base fee que foi queimada;
- priority fee creditada ao produtor correto;
- interação com as recompensas bloqueadas do mesmo bloco;
- mesmo bloco e receipt observados pelos 20 nós.

Em seguida, o script executa novamente o auditor de rewards até a ponta da cadeia. Os
relatórios JSON e Markdown e os dados do auditor de rewards são reunidos em um único arquivo
`rabbit-transaction-audit-result-*.tar.gz`.

## Execução

Primeiro compile e execute os testes dos módulos. Depois inicie novamente o laboratório com
`scripts/rabbit-devnet/start-rabbit-20producers.sh` e execute
`scripts/rabbit-devnet/run-transaction-audit.sh`.

Este teste é uma linha de base para o engine atualmente ativo. A troca de `lqcv2` para `lqc`
não faz parte deste pacote e só deve acontecer depois da auditoria de prontidão do LQC.
