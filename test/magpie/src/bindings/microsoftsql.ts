import { Binding, BindingStatus} from '../binding'
import sql from 'mssql';

// Use this with a values like: CONNECTION_SQL_CONNECTIONSTRING
export class MicrosoftSqlBinding implements Binding {
    private connectionString: string;

    constructor(map: { [key: string]: string }) {
        this.connectionString = map['CONNECTIONSTRING'];
        if (!this.connectionString) {
            throw new Error('connectionString is required');
        }
    }

    public async status(): Promise<BindingStatus> {
        // Type definitions in the library are wrong. Strings are allowed here.
        let connection = await sql.connect(this.connectionString as unknown as sql.config)
        await connection.query('select 1 as number')
        return { ok: true, message: "database accessed"};
    }

    public toString = () : string => {
        return 'Microsoft SQL';
    }
}