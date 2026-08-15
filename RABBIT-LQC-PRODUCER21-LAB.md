# Rabbit Chain — laboratório permissionless do produtor 21

Este laboratório cria uma blockchain temporária nova em
`/tmp/rabbit-permissionless-21nodes`. Vinte endereços formam apenas o conjunto
inicial do genesis. O endereço do `node21` não aparece no genesis, não recebe
RAB e conecta-se a somente três peers, simulando um usuário externo.

O protocolo canônico é ativado no bloco 1. O auditor executa automaticamente:

1. convergência inicial dos 21 nós;
2. confirmação de que o node21 não está cadastrado e possui saldo zero;
3. REGISTER com LightHash e assinatura local;
4. inclusão canônica e elegibilidade em 21/21 nós;
5. produção de um bloco pelo novo participante;
6. HEARTBEAT assinado;
7. EXIT assinado e 30 blocos sem produção pelo endereço inativo;
8. novo REGISTER sem administrador;
9. nova produção após o retorno;
10. convergência e distribuição finais.

As chaves privadas e a senha não entram no relatório. O laboratório padrão de
20 nós permanece com a ativação desabilitada. O auditor interrompe os processos
desse laboratório antigo para evitar uso excessivo de memória, mas não apaga os
bancos antigos.

O teste exige pelo menos 20 GiB livres tanto no disco virtual do WSL quanto no
disco C: que armazena esse disco virtual. Se a trava falhar, nenhum banco novo é
criado.
