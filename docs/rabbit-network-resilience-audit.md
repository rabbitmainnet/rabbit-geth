# Rabbit Chain — auditoria de resiliência do laboratório

O script `scripts/rabbit-devnet/run-network-resilience-audit.sh` testa o
laboratório já inicializado sem alterar os arquivos de genesis oficiais e sem
alterar o consenso.

Ele executa, nesta ordem:

1. convergência inicial dos 20 nós;
2. três transações EIP-1559 sequenciais;
3. desligamento de sete produtores;
4. produção de blocos e uma transação com 13 produtores online;
5. retorno e sincronização dos sete produtores;
6. uma transação após o retorno;
7. desligamento limpo dos 20 nós;
8. reinício dos mesmos bancos de dados e validação do checkpoint anterior;
9. retomada da produção e uma nova transação;
10. auditoria final de recompensas e reward locker.

Em qualquer erro ou interrupção, o manipulador de saída tenta religar e
reconectar os 20 nós. O resultado é salvo em `audit-reports` e empacotado em um
único arquivo `rabbit-network-resilience-result-*.tar.gz`.

O teste é exclusivo do laboratório `/tmp/rabbit-20nodes`. As contas, o saldo
líquido de teste e o genesis runtime continuam fora da configuração oficial da
Rabbit Chain.

Desde a versão 1.0.1, o teste também valida a recuperação de forks usando o
downloader completo do cliente. A recuperação encontra o ancestral comum e
transfere blocos completos, incluindo transações e receipts; ela não cria
blocos vazios a partir de cabeçalhos remotos.
