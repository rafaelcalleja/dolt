// Copyright 2019 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package json

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/dolthub/go-mysql-server/sql"
	"github.com/dolthub/vitess/go/sqltypes"

	"github.com/dolthub/dolt/go/libraries/doltcore/row"
	"github.com/dolthub/dolt/go/libraries/doltcore/schema"
	"github.com/dolthub/dolt/go/libraries/doltcore/schema/typeinfo"
	"github.com/dolthub/dolt/go/libraries/doltcore/table"
	"github.com/dolthub/dolt/go/libraries/utils/iohelp"
	"github.com/dolthub/dolt/go/store/types"
)

const jsonHeader = `{"rows": [`
const jsonFooter = `]}`

var WriteBufSize = 256 * 1024
var defaultString = sql.MustCreateStringWithDefaults(sqltypes.VarChar, 16383)

type RowWriter struct {
	closer      io.Closer
	header      string
	footer      string
	separator   string
	bWr         *bufio.Writer
	sch         schema.Schema
	rowsWritten int
}

var _ table.SqlRowWriter = (*RowWriter)(nil)

// NewJSONWriter returns a new writer that encodes rows as a single JSON object with a single key: "rows", which is a
// slice of all rows. To customize the output of the JSON object emitted, use |NewJSONWriterWithHeader|
func NewJSONWriter(wr io.WriteCloser, outSch schema.Schema) (*RowWriter, error) {
	return NewJSONWriterWithHeader(wr, outSch, jsonHeader, jsonFooter, ",")
}

func NewJSONWriterWithHeader(wr io.WriteCloser, outSch schema.Schema, header, footer, separator string) (*RowWriter, error) {
	bwr := bufio.NewWriterSize(wr, WriteBufSize)
	return &RowWriter{
		closer:    wr,
		bWr:       bwr,
		sch:       outSch,
		header:    header,
		footer:    footer,
		separator: separator,
	}, nil
}

func (j *RowWriter) GetSchema() schema.Schema {
	return j.sch
}

// WriteRow encodes the row given into JSON format and writes it, returning any error
func (j *RowWriter) WriteRow(ctx context.Context, r row.Row) error {
	if j.rowsWritten == 0 {
		err := iohelp.WriteAll(j.bWr, []byte(j.header))
		if err != nil {
			return err
		}
	}

	allCols := j.sch.GetAllCols()
	colValMap := make(map[string]interface{}, allCols.Size())
	if err := allCols.Iter(func(tag uint64, col schema.Column) (stop bool, err error) {
		val, ok := r.GetColVal(tag)
		if !ok || types.IsNull(val) {
			return false, nil
		}

		switch col.TypeInfo.GetTypeIdentifier() {
		case typeinfo.DatetimeTypeIdentifier,
			typeinfo.DecimalTypeIdentifier,
			typeinfo.EnumTypeIdentifier,
			typeinfo.InlineBlobTypeIdentifier,
			typeinfo.SetTypeIdentifier,
			typeinfo.TimeTypeIdentifier,
			typeinfo.TupleTypeIdentifier,
			typeinfo.UuidTypeIdentifier,
			typeinfo.VarBinaryTypeIdentifier,
			typeinfo.YearTypeIdentifier:
			v, err := col.TypeInfo.FormatValue(val)
			if err != nil {
				return true, err
			}
			val = types.String(*v)

		case typeinfo.BitTypeIdentifier,
			typeinfo.BoolTypeIdentifier,
			typeinfo.VarStringTypeIdentifier,
			typeinfo.UintTypeIdentifier,
			typeinfo.IntTypeIdentifier,
			typeinfo.FloatTypeIdentifier:
			// use primitive type
		}

		colValMap[col.Name] = val

		return false, nil
	}); err != nil {
		return err
	}

	data, err := marshalToJson(colValMap)
	if err != nil {
		return errors.New("marshaling did not work")
	}

	if j.rowsWritten != 0 {
		_, err := j.bWr.WriteString(j.separator)
		if err != nil {
			return err
		}
	}

	newErr := iohelp.WriteAll(j.bWr, data)
	if newErr != nil {
		return newErr
	}
	j.rowsWritten++

	return nil
}

func (j *RowWriter) WriteSqlRow(ctx context.Context, row sql.Row) error {
	if j.rowsWritten == 0 {
		err := iohelp.WriteAll(j.bWr, []byte(j.header))
		if err != nil {
			return err
		}
	}

	allCols := j.sch.GetAllCols()
	colValMap := make(map[string]interface{}, allCols.Size())
	if err := allCols.Iter(func(tag uint64, col schema.Column) (stop bool, err error) {
		val := row[allCols.TagToIdx[tag]]
		if val == nil {
			return false, nil
		}

		switch col.TypeInfo.GetTypeIdentifier() {
		case typeinfo.DatetimeTypeIdentifier,
			typeinfo.DecimalTypeIdentifier,
			typeinfo.EnumTypeIdentifier,
			typeinfo.InlineBlobTypeIdentifier,
			typeinfo.SetTypeIdentifier,
			typeinfo.TimeTypeIdentifier,
			typeinfo.TupleTypeIdentifier,
			typeinfo.UuidTypeIdentifier,
			typeinfo.VarBinaryTypeIdentifier:
			sqlVal, err := col.TypeInfo.ToSqlType().SQL(nil, val)
			if err != nil {
				return true, err
			}
			val = sqlVal.ToString()

		case typeinfo.BitTypeIdentifier,
			typeinfo.BoolTypeIdentifier,
			typeinfo.VarStringTypeIdentifier,
			typeinfo.UintTypeIdentifier,
			typeinfo.IntTypeIdentifier,
			typeinfo.FloatTypeIdentifier,
			typeinfo.YearTypeIdentifier:
			// use primitive type
		}

		colValMap[col.Name] = val

		return false, nil
	}); err != nil {
		return err
	}

	data, err := marshalToJson(colValMap)
	if err != nil {
		return errors.New("marshaling did not work")
	}

	if j.rowsWritten != 0 {
		_, err := j.bWr.WriteString(j.separator)
		if err != nil {
			return err
		}
	}

	newErr := iohelp.WriteAll(j.bWr, data)
	if newErr != nil {
		return newErr
	}
	j.rowsWritten++

	return nil
}

func (j *RowWriter) Flush() error {
	return j.bWr.Flush()
}

// Close should flush all writes, release resources being held
func (j *RowWriter) Close(ctx context.Context) error {
	if j.closer != nil {
		if j.rowsWritten > 0 {
			err := iohelp.WriteAll(j.bWr, []byte(j.footer))
			if err != nil {
				return err
			}
		}

		errFl := j.bWr.Flush()
		errCl := j.closer.Close()
		j.closer = nil

		if errCl != nil {
			return errCl
		}

		return errFl
	}

	return errors.New("already closed")
}

func marshalToJson(valMap interface{}) ([]byte, error) {
	var jsonBytes []byte
	var err error

	jsonBytes, err = json.Marshal(valMap)
	if err != nil {
		return nil, err
	}
	return jsonBytes, nil
}
